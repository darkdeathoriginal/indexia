package service

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"indexia/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

const MaxMessageLength = 3800 // Safety buffer below Telegram 4096 char limit
const ChannelPostDelay = 1500 * time.Millisecond // Telegram channel rate limit pacing (max ~20 msgs/min)
const ChannelEditDelay = 250 * time.Millisecond  // Pacing for message edits

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
	var alphabetPages []PreparedPage
	for _, letter := range alphabetOrder {
		items := grouped[letter]
		letterPages := s.buildAlphabetPages(letter, items)
		alphabetPages = append(alphabetPages, letterPages...)
	}

	// Fetch existing channel message tracking from DB
	var existingMsgs []models.ChannelMessage
	if err := s.db.Where("channel_id = ?", channelID).Order("order_index ASC").Find(&existingMsgs).Error; err != nil {
		return fmt.Errorf("failed to fetch channel message records: %w", err)
	}

	// Separate existing tracked messages into alphabet messages and footer messages
	var alphabetMsgs []models.ChannelMessage
	var footerMsgs []models.ChannelMessage

	for _, m := range existingMsgs {
		if m.MessageType == "footer" {
			footerMsgs = append(footerMsgs, m)
		} else {
			alphabetMsgs = append(alphabetMsgs, m)
		}
	}

	reqAlphabetCount := len(alphabetPages)
	currentAlphabetCount := len(alphabetMsgs)

	var footers []models.FooterMessage
	if err := s.db.Order("order_index ASC").Find(&footers).Error; err != nil {
		log.Printf("Error fetching footers: %v", err)
	}

	// Cascading / Footer Re-post Trigger:
	// Only delete and re-post footers if:
	// 1. Alphabet message count changed (cascading happened!)
	// 2. OR number of footer messages changed (a footer was added/deleted)
	needsFooterRepost := (reqAlphabetCount != currentAlphabetCount) || (len(footerMsgs) != len(footers))

	if needsFooterRepost && len(footerMsgs) > 0 {
		log.Println("Cascading / Footer change detected: Deleting existing footer messages to re-post at bottom.")
		for _, m := range footerMsgs {
			s.deleteTelegramMessage(channelID, m.MessageID)
		}
		s.db.Where("channel_id = ? AND message_type = ?", channelID, "footer").Delete(&models.ChannelMessage{})
	}

	// Adjust total alphabet message count in Telegram
	if reqAlphabetCount > currentAlphabetCount {
		needed := reqAlphabetCount - currentAlphabetCount
		log.Printf("Posting %d new message(s) to Telegram channel to expand capacity...", needed)

		for i := 0; i < needed; i++ {
			msgConfig := tgbotapi.NewMessage(channelID, "⏳ <i>Initializing directory index...</i>")
			msgConfig.ParseMode = "HTML"
			sentMsg, err := s.sendTelegramMessage(msgConfig)
			if err != nil {
				return fmt.Errorf("failed to send initial channel message (%d/%d): %w", i+1, needed, err)
			}

			newMsgRecord := models.ChannelMessage{
				ChannelID:   channelID,
				MessageID:   sentMsg.MessageID,
				MessageType: "alphabet",
				OrderIndex:  currentAlphabetCount + i,
				ContentHash: "",
			}
			s.db.Create(&newMsgRecord)
			alphabetMsgs = append(alphabetMsgs, newMsgRecord)
			time.Sleep(ChannelPostDelay)
		}
	} else if reqAlphabetCount < currentAlphabetCount {
		surplusCount := currentAlphabetCount - reqAlphabetCount
		log.Printf("Deleting %d surplus message(s) from Telegram channel...", surplusCount)

		for i := currentAlphabetCount - 1; i >= reqAlphabetCount; i-- {
			msgToDelete := alphabetMsgs[i]
			s.deleteTelegramMessage(channelID, msgToDelete.MessageID)
			s.db.Delete(&msgToDelete)
			time.Sleep(ChannelEditDelay)
		}
		alphabetMsgs = alphabetMsgs[:reqAlphabetCount]
	}

	// Update all alphabet channel messages with current page contents
	for idx, page := range alphabetPages {
		hash := hashContent(page.Content)
		msgRecord := &alphabetMsgs[idx]

		if msgRecord.ContentHash != hash || msgRecord.MessageType != page.Type || msgRecord.Alphabet != page.Alphabet || msgRecord.Part != page.Part {
			log.Printf("Updating alphabet message %d (Index %d/%d, Alphabet: %s, Part: %d)...",
				msgRecord.MessageID, idx+1, len(alphabetPages), page.Alphabet, page.Part)

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
			time.Sleep(ChannelEditDelay)
		}
	}

	// Re-post footer messages ONLY when cascading happened or footers changed/don't exist
	if needsFooterRepost || len(footerMsgs) == 0 {
		if len(footers) > 0 {
			log.Printf("Posting %d footer message(s) at bottom of channel...", len(footers))
			for idx, footer := range footers {
				sentMsg, err := s.sendFooterTelegramMessage(channelID, footer)
				if err != nil {
					log.Printf("Error sending footer #%d (type: %s): %v", footer.ID, footer.MessageType, err)
					continue
				}

				footer.TelegramMsgID = sentMsg.MessageID
				s.db.Save(&footer)

				footerRecord := models.ChannelMessage{
					ChannelID:   channelID,
					MessageID:   sentMsg.MessageID,
					MessageType: "footer",
					OrderIndex:  reqAlphabetCount + idx,
					ContentHash: hashContent(footer.Content + footer.FileID),
				}
				s.db.Create(&footerRecord)
				time.Sleep(ChannelPostDelay)
			}
		}
	} else {
		log.Println("No cascading or footer change: Footers preserved in place at bottom of channel.")
	}

	log.Println("Channel sync completed successfully.")
	return nil
}

func (s *SyncService) sendFooterTelegramMessage(channelID int64, footer models.FooterMessage) (tgbotapi.Message, error) {
	var chattable tgbotapi.Chattable

	switch footer.MessageType {
	case "sticker":
		msg := tgbotapi.NewSticker(channelID, tgbotapi.FileID(footer.FileID))
		chattable = msg
	case "photo":
		msg := tgbotapi.NewPhoto(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	case "video":
		msg := tgbotapi.NewVideo(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	case "animation":
		msg := tgbotapi.NewAnimation(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	case "document":
		msg := tgbotapi.NewDocument(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	case "audio":
		msg := tgbotapi.NewAudio(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	case "voice":
		msg := tgbotapi.NewVoice(channelID, tgbotapi.FileID(footer.FileID))
		if footer.Content != "" {
			msg.Caption = footer.Content
			msg.ParseMode = "HTML"
		}
		chattable = msg
	default: // "text"
		msg := tgbotapi.NewMessage(channelID, footer.Content)
		msg.ParseMode = "HTML"
		msg.DisableWebPagePreview = true
		chattable = msg
	}

	for retries := 0; retries < 10; retries++ {
		msg, err := s.bot.Send(chattable)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				delay := getRetryDelay(err, retries)
				log.Printf("⚠️ Rate limited by Telegram sending footer. Waiting %v before retry %d/10...", delay, retries+1)
				time.Sleep(delay)
				continue
			}
			return msg, err
		}
		return msg, nil
	}
	return tgbotapi.Message{}, fmt.Errorf("failed after 10 rate limit retries")
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
		safeName := html.EscapeString(entry.Name)
		safeURL := html.EscapeString(entry.URL)
		line := fmt.Sprintf("• <a href=\"%s\">%s</a>", safeURL, safeName)
		lineLen := len(line) + 1

		if currentLen+lineLen > MaxMessageLength && len(currentLines) > 0 {
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

		pages = append(pages, PreparedPage{
			Type:     "alphabet",
			Alphabet: letter,
			Part:     part,
			Content:  header + body,
		})
	}

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
	return 0, fmt.Errorf("channel ID should be a numeric ID (e.g., -100123456789). Telegram username lookups require numeric channel ID")
}

func getRetryDelay(err error, retryAttempt int) time.Duration {
	if apiErr, ok := err.(tgbotapi.Error); ok {
		if apiErr.ResponseParameters.RetryAfter > 0 {
			return time.Duration(apiErr.ResponseParameters.RetryAfter+2) * time.Second
		}
	}
	re := regexp.MustCompile(`retry after (\d+)`)
	matches := re.FindStringSubmatch(err.Error())
	if len(matches) > 1 {
		if sec, parseErr := strconv.Atoi(matches[1]); parseErr == nil && sec > 0 {
			return time.Duration(sec+2) * time.Second
		}
	}
	return time.Duration((retryAttempt+1)*3) * time.Second
}

func (s *SyncService) sendTelegramMessage(config tgbotapi.MessageConfig) (tgbotapi.Message, error) {
	for retries := 0; retries < 10; retries++ {
		msg, err := s.bot.Send(config)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				delay := getRetryDelay(err, retries)
				log.Printf("⚠️ Rate limited by Telegram when sending message. Waiting %v before retry %d/10...", delay, retries+1)
				time.Sleep(delay)
				continue
			}
			return msg, err
		}
		return msg, nil
	}
	return tgbotapi.Message{}, fmt.Errorf("failed after 10 rate limit retries")
}

func (s *SyncService) editTelegramMessage(config tgbotapi.EditMessageTextConfig) error {
	for retries := 0; retries < 10; retries++ {
		_, err := s.bot.Request(config)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				delay := getRetryDelay(err, retries)
				log.Printf("⚠️ Rate limited by Telegram when editing message %d. Waiting %v before retry %d/10...", config.MessageID, delay, retries+1)
				time.Sleep(delay)
				continue
			}
			if strings.Contains(err.Error(), "message is not modified") {
				return nil
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("failed to edit message after 10 retries")
}

func (s *SyncService) deleteTelegramMessage(channelID int64, messageID int) {
	deleteConfig := tgbotapi.NewDeleteMessage(channelID, messageID)
	for retries := 0; retries < 5; retries++ {
		_, err := s.bot.Request(deleteConfig)
		if err != nil {
			if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "429") {
				delay := getRetryDelay(err, retries)
				time.Sleep(delay)
				continue
			}
			log.Printf("Warning: failed to delete message %d: %v", messageID, err)
			break
		}
		break
	}
}
