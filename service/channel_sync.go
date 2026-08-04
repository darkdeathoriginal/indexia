package service

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"indexia/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

const MaxMessageLength = 3800 // Safety buffer below Telegram 4096 char limit

type SyncService struct {
	db        *gorm.DB
	bot       *tgbotapi.BotAPI
	syncMutex sync.Mutex
}

func NewSyncService(db *gorm.DB, bot *tgbotapi.BotAPI) *SyncService {
	return &SyncService{
		db:  db,
		bot: bot,
	}
}

type PreparedPage struct {
	Type     string // "alphabet" or "footer"
	Alphabet string // "A", "B", etc.
	Part     int    // 1, 2...
	Content  string // HTML string
}

// SyncChannel synchronizes all entries and footer messages with the Telegram channel
func (s *SyncService) SyncChannel(channelIDStr string) error {
	s.syncMutex.Lock()
	defer s.syncMutex.Unlock()

	if channelIDStr == "" {
		// Try fetching from settings
		var setting models.Setting
		if err := s.db.First(&setting, "key = ?", "channel_id").Error; err != nil {
			return fmt.Errorf("channel ID not configured. Use /setchannel <channel_id_or_username>")
		}
		channelIDStr = setting.Value
	}

	channelID, err := parseChannelID(channelIDStr)
	if err != nil {
		return fmt.Errorf("invalid channel ID format: %w", err)
	}

	// 1. Fetch all entries sorted by Name
	var entries []models.Entry
	if err := s.db.Where("deleted_at IS NULL").Order("name ASC").Find(&entries).Error; err != nil {
		return fmt.Errorf("failed to fetch entries: %w", err)
	}

	// Group entries by Alphabet (A-Z, #)
	grouped := make(map[string][]models.Entry)
	alphabetOrder := getAlphabetList()

	for _, entry := range entries {
		letter := models.GetFirstLetter(entry.Name)
		grouped[letter] = append(grouped[letter], entry)
	}

	// Build prepared pages for alphabets
	var pages []PreparedPage

	for _, letter := range alphabetOrder {
		items := grouped[letter]
		letterPages := s.buildAlphabetPages(letter, items)
		pages = append(pages, letterPages...)
	}

	// Fetch footer messages
	var footers []models.FooterMessage
	if err := s.db.Order("order_index ASC").Find(&footers).Error; err != nil {
		return fmt.Errorf("failed to fetch footers: %w", err)
	}

	var footerPages []PreparedPage
	for idx, footer := range footers {
		footerPages = append(footerPages, PreparedPage{
			Type:     "footer",
			Alphabet: "",
			Part:     idx + 1,
			Content:  footer.Content,
		})
	}

	// Total required pages = Alphabet Pages + Footer Pages
	allRequiredPages := append(pages, footerPages...)

	// Fetch existing channel message tracking from DB
	var existingMsgs []models.ChannelMessage
	if err := s.db.Where("channel_id = ?", channelID).Order("order_index ASC").Find(&existingMsgs).Error; err != nil {
		return fmt.Errorf("failed to fetch channel message records: %w", err)
	}

	log.Printf("Syncing channel %d: %d required pages (%d alphabet, %d footer), %d existing messages in DB",
		channelID, len(allRequiredPages), len(pages), len(footerPages), len(existingMsgs))

	// Check if cascading is required (i.e., required alphabet pages expanded or footer position needs delete/repost)
	// Cascading Rule:
	// If alphabet page count expanded, existing footer messages at bottom MUST be deleted from Telegram
	// and newly created at the bottom after cascading alphabet pages.
	alphabetPageCount := len(pages)
	prevAlphabetMsgCount := 0
	for _, m := range existingMsgs {
		if m.MessageType == "alphabet" {
			prevAlphabetMsgCount++
		}
	}

	needsCascading := alphabetPageCount > prevAlphabetMsgCount

	if needsCascading {
		log.Println("Cascading triggered: Alphabet page count increased. Deleting existing footer messages to re-post at bottom.")
		// Delete any existing footer messages in Telegram
		for _, m := range existingMsgs {
			if m.MessageType == "footer" {
				s.deleteTelegramMessage(channelID, m.MessageID)
			}
		}
		// Clear footer tracked messages from DB so they get re-created below
		s.db.Where("channel_id = ? AND message_type = ?", channelID, "footer").Delete(&models.ChannelMessage{})

		// Re-fetch existing msgs (now only alphabet msgs)
		s.db.Where("channel_id = ?", channelID).Order("order_index ASC").Find(&existingMsgs)
	}

	// Adjust total message count in Telegram to match allRequiredPages count
	reqCount := len(allRequiredPages)
	currentCount := len(existingMsgs)

	if reqCount > currentCount {
		// Post additional new messages to channel
		needed := reqCount - currentCount
		log.Printf("Posting %d new message(s) to Telegram channel to expand capacity...", needed)

		for i := 0; i < needed; i++ {
			msgConfig := tgbotapi.NewMessage(channelID, "⏳ <i>Initializing directory index...</i>")
			msgConfig.ParseMode = "HTML"
			sentMsg, err := s.sendTelegramMessage(msgConfig)
			if err != nil {
				return fmt.Errorf("failed to send initial channel message: %w", err)
			}

			newMsgRecord := models.ChannelMessage{
				ChannelID:   channelID,
				MessageID:   sentMsg.MessageID,
				MessageType: "placeholder",
				OrderIndex:  currentCount + i,
				ContentHash: "",
			}
			s.db.Create(&newMsgRecord)
			existingMsgs = append(existingMsgs, newMsgRecord)
			time.Sleep(100 * time.Millisecond) // smooth out API requests
		}
	} else if reqCount < currentCount {
		// Delete surplus messages from the end of the channel
		surplusCount := currentCount - reqCount
		log.Printf("Deleting %d surplus message(s) from Telegram channel...", surplusCount)

		for i := currentCount - 1; i >= reqCount; i-- {
			msgToDelete := existingMsgs[i]
			s.deleteTelegramMessage(channelID, msgToDelete.MessageID)
			s.db.Delete(&msgToDelete)
			time.Sleep(100 * time.Millisecond)
		}
		existingMsgs = existingMsgs[:reqCount]
	}

	// Update all channel messages with current page contents
	for idx, page := range allRequiredPages {
		hash := hashContent(page.Content)
		msgRecord := &existingMsgs[idx]

		if msgRecord.ContentHash != hash || msgRecord.MessageType != page.Type || msgRecord.Alphabet != page.Alphabet || msgRecord.Part != page.Part {
			log.Printf("Updating message %d (Index %d, Type: %s, Alphabet: %s, Part: %d)...",
				msgRecord.MessageID, idx, page.Type, page.Alphabet, page.Part)

			editMsg := tgbotapi.NewEditMessageText(channelID, msgRecord.MessageID, page.Content)
			editMsg.ParseMode = "HTML"
			editMsg.DisableWebPagePreview = true

			err := s.editTelegramMessage(editMsg)
			if err != nil {
				log.Printf("Error editing message %d: %v", msgRecord.MessageID, err)
			} else {
				msgRecord.MessageType = page.Type
				msgRecord.Alphabet = page.Alphabet
				msgRecord.Part = page.Part
				msgRecord.ContentHash = hash
				s.db.Save(msgRecord)
			}
			time.Sleep(150 * time.Millisecond)
		}
	}

	log.Println("Channel sync completed successfully.")
	return nil
}

func (s *SyncService) buildAlphabetPages(letter string, entries []models.Entry) []PreparedPage {
	if len(entries) == 0 {
		content := fmt.Sprintf("<b>🔤 ALPHABET: %s</b>\n\n<i>(No links listed under %s)</i>", letter, letter)
		return []PreparedPage{
			{
				Type:     "alphabet",
				Alphabet: letter,
				Part:     1,
				Content:  content,
			},
		}
	}

	var pages []PreparedPage
	part := 1
	var currentLines []string
	currentLen := 0

	getHeader := func(p int, multi bool) string {
		if multi {
			return fmt.Sprintf("<b>🔤 ALPHABET: %s (Part %d)</b>\n\n", letter, p)
		}
		return fmt.Sprintf("<b>🔤 ALPHABET: %s</b>\n\n", letter)
	}

	for _, entry := range entries {
		// Escape HTML special chars in name and URL
		safeName := html.EscapeString(entry.Name)
		safeURL := html.EscapeString(entry.URL)
		line := fmt.Sprintf("• <a href=\"%s\">%s</a>", safeURL, safeName)

		lineLen := len(line) + 1 // including newline

		if currentLen+lineLen > MaxMessageLength && len(currentLines) > 0 {
			// Save current part
			header := getHeader(part, true)
			body := strings.Join(currentLines, "\n")
			pages = append(pages, PreparedPage{
				Type:     "alphabet",
				Alphabet: letter,
				Part:     part,
				Content:  header + body,
			})
			part++
			currentLines = nil
			currentLen = 0
		}

		currentLines = append(currentLines, line)
		currentLen += lineLen
	}

	if len(currentLines) > 0 {
		isMulti := part > 1
		header := getHeader(part, isMulti)
		body := strings.Join(currentLines, "\n")

		// If it's multi-part, fix header of part 1 if previously created as single part
		pages = append(pages, PreparedPage{
			Type:     "alphabet",
			Alphabet: letter,
			Part:     part,
			Content:  header + body,
		})
	}

	// If there are multiple parts, adjust part 1 header if needed
	if len(pages) > 1 {
		pages[0].Content = strings.Replace(pages[0].Content,
			fmt.Sprintf("<b>🔤 ALPHABET: %s</b>\n\n", letter),
			fmt.Sprintf("<b>🔤 ALPHABET: %s (Part 1)</b>\n\n", letter), 1)
	}

	return pages
}

func getAlphabetList() []string {
	list := make([]string, 0, 27)
	for c := 'A'; c <= 'Z'; c++ {
		list = append(list, string(c))
	}
	list = append(list, "#")
	return list
}

func hashContent(content string) string {
	h := md5.Sum([]byte(content))
	return hex.EncodeToString(h[:])
}

func parseChannelID(str string) (int64, error) {
	str = strings.TrimSpace(str)
	if id, err := strconv.ParseInt(str, 10, 64); err == nil {
		return id, nil
	}
	return 0, fmt.Errorf("channel ID should be a numeric ID (e.g., -100123456789). Telegram username lookups require channel ID")
}

func (s *SyncService) sendTelegramMessage(config tgbotapi.MessageConfig) (tgbotapi.Message, error) {
	for retries := 0; retries < 5; retries++ {
		msg, err := s.bot.Send(config)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				time.Sleep(time.Duration(retries+1) * 2 * time.Second)
				continue
			}
			return msg, err
		}
		return msg, nil
	}
	return tgbotapi.Message{}, fmt.Errorf("failed after retries")
}

func (s *SyncService) editTelegramMessage(config tgbotapi.EditMessageTextConfig) error {
	for retries := 0; retries < 5; retries++ {
		_, err := s.bot.Request(config)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				time.Sleep(time.Duration(retries+1) * 2 * time.Second)
				continue
			}
			// If message content is unchanged, Telegram returns an error which we can safely ignore
			if strings.Contains(err.Error(), "message is not modified") {
				return nil
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("failed to edit message after retries")
}

func (s *SyncService) deleteTelegramMessage(channelID int64, messageID int) {
	deleteConfig := tgbotapi.NewDeleteMessage(channelID, messageID)
	for retries := 0; retries < 3; retries++ {
		_, err := s.bot.Request(deleteConfig)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				time.Sleep(time.Duration(retries+1) * 2 * time.Second)
				continue
			}
			log.Printf("Warning: failed to delete message %d: %v", messageID, err)
			break
		}
		break
	}
}
