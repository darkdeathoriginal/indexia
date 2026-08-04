package bot

import (
	"fmt"
	"html"
	"log"
	"net/url"
	"strconv"
	"strings"

	"indexia/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	user, justPromoted := b.ensureUser(msg.From)
	if justPromoted {
		b.replyHTML(msg.Chat.ID, fmt.Sprintf(
			"👑 <b>Setup Complete!</b>\n\nWelcome %s! You have been configured as the <b>Initial Super Admin</b> of this bot.\n\nUse /help to see all available admin commands.",
			html.EscapeString(msg.From.FirstName),
		))
	}

	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			b.handleHelp(msg, user)
		case "add":
			b.handleAdd(msg, user)
		case "delete", "del", "remove":
			b.handleDelete(msg, user)
		case "list":
			b.handleList(msg, user)
		case "setchannel":
			b.handleSetChannel(msg, user)
		case "addadmin":
			b.handleAddAdmin(msg, user)
		case "removeadmin":
			b.handleRemoveAdmin(msg, user)
		case "admins":
			b.handleListAdmins(msg, user)
		case "addfooter", "setfooter":
			b.handleAddFooter(msg, user)
		case "clearfooters":
			b.handleClearFooters(msg, user)
		case "sync":
			b.handleSync(msg, user)
		default:
			b.replyText(msg.Chat.ID, "Unknown command. Send /help for available commands.")
		}
	} else if user.IsAdmin {
		// Non-command text message: check if it's "Name | URL" format for quick add
		if strings.Contains(msg.Text, "|") {
			b.handleAddInline(msg, user, msg.Text)
		} else {
			b.replyText(msg.Chat.ID, "💡 Tip: Send an entry in the format `Name | https://link.com` to add it, or type /help.")
		}
	}
}

func (b *Bot) ensureUser(from *tgbotapi.User) (*models.User, bool) {
	if from == nil {
		return nil, false
	}

	var user models.User
	err := b.db.Where("telegram_id = ?", from.ID).First(&user).Error

	justPromoted := false

	if err != nil {
		// User does not exist in DB yet
		var adminCount int64
		b.db.Model(&models.User{}).Where("is_admin = ?", true).Count(&adminCount)

		isAdmin := adminCount == 0
		user = models.User{
			TelegramID:   from.ID,
			Username:     from.UserName,
			FirstName:    from.FirstName,
			IsAdmin:      isAdmin,
			IsSuperAdmin: isAdmin,
		}
		b.db.Create(&user)
		justPromoted = isAdmin
	} else {
		// Update username/firstname if changed
		if user.Username != from.UserName || user.FirstName != from.FirstName {
			user.Username = from.UserName
			user.FirstName = from.FirstName
			b.db.Save(&user)
		}
	}

	return &user, justPromoted
}

func (b *Bot) handleHelp(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyHTML(msg.Chat.ID, "👋 Welcome to <b>Alphabetical Channel Directory Bot</b>!\n\nContact an admin to get access.")
		return
	}

	text := `🤖 <b>Alphabetical Directory Bot - Admin Panel</b>

<b>Entry Management:</b>
• <code>/add Name | https://link.com</code> - Add a new entry (or send <i>Name | URL</i>)
• <code>/delete &lt;ID&gt;</code> - Delete entry by ID
• <code>/list</code> - List all entries in database

<b>Channel & Footer Settings:</b>
• <code>/setchannel &lt;ChannelID&gt;</code> - Set channel ID (e.g. -1001234567890)
• <code>/addfooter &lt;text&gt;</code> - Add footer message (or reply to a message with /addfooter)
• <code>/clearfooters</code> - Clear all footer messages
• <code>/sync</code> - Force trigger channel update & cascading sync

<b>Admin Management:</b>
• <code>/addadmin &lt;UserID or Username&gt;</code> - Grant admin rights
• <code>/removeadmin &lt;UserID or Username&gt;</code> - Revoke admin rights
• <code>/admins</code> - List all current admins`

	b.replyHTML(msg.Chat.ID, text)
}

func (b *Bot) handleAdd(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied. Admin access required.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b>\n<code>/add Name | https://example.com</code>\n\nOr simply send: <code>Name | https://example.com</code>")
		return
	}

	b.handleAddInline(msg, user, args)
}

func (b *Bot) handleAddInline(msg *tgbotapi.Message, user *models.User, text string) {
	parts := strings.SplitN(text, "|", 2)
	if len(parts) < 2 {
		b.replyHTML(msg.Chat.ID, "⚠️ Invalid format. Use: <code>Name | https://link.com</code>")
		return
	}

	name := strings.TrimSpace(parts[0])
	rawURL := strings.TrimSpace(parts[1])

	if name == "" || rawURL == "" {
		b.replyText(msg.Chat.ID, "❌ Both Name and URL are required.")
		return
	}

	// Validate URL format
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "t.me/") {
		rawURL = "https://" + rawURL
	}

	_, err := url.ParseRequestURI(rawURL)
	if err != nil {
		b.replyText(msg.Chat.ID, fmt.Sprintf("❌ Invalid URL: %s", rawURL))
		return
	}

	letter := models.GetFirstLetter(name)

	entry := models.Entry{
		Name:        name,
		URL:         rawURL,
		FirstLetter: letter,
		AddedByID:   user.TelegramID,
	}

	if err := b.db.Create(&entry).Error; err != nil {
		b.replyText(msg.Chat.ID, fmt.Sprintf("❌ Failed to save entry: %v", err))
		return
	}

	b.replyHTML(msg.Chat.ID, fmt.Sprintf("✅ <b>Entry Added!</b>\n\n<b>ID:</b> %d\n<b>Name:</b> %s\n<b>Letter:</b> %s\n<b>URL:</b> %s\n\n<i>Syncing channel...</i>",
		entry.ID, html.EscapeString(entry.Name), entry.FirstLetter, html.EscapeString(entry.URL)))

	// Automatically sync channel
	go func() {
		if err := b.syncSvc.SyncChannel(b.cfg.ChannelID); err != nil {
			log.Printf("Auto-sync error after add: %v", err)
			b.replyText(msg.Chat.ID, fmt.Sprintf("⚠️ Entry saved, but channel sync warning: %v", err))
		}
	}()
}

func (b *Bot) handleDelete(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b> <code>/delete &lt;ID&gt;</code>")
		return
	}

	id, err := strconv.ParseUint(args, 10, 64)
	if err != nil {
		b.replyText(msg.Chat.ID, "❌ Invalid ID. Please specify a numeric entry ID.")
		return
	}

	var entry models.Entry
	if err := b.db.First(&entry, id).Error; err != nil {
		b.replyText(msg.Chat.ID, fmt.Sprintf("❌ Entry with ID %d not found.", id))
		return
	}

	b.db.Delete(&entry)

	b.replyHTML(msg.Chat.ID, fmt.Sprintf("🗑️ <b>Entry #%d deleted:</b> %s\n\n<i>Syncing channel...</i>", entry.ID, html.EscapeString(entry.Name)))

	go func() {
		if err := b.syncSvc.SyncChannel(b.cfg.ChannelID); err != nil {
			log.Printf("Auto-sync error after delete: %v", err)
		}
	}()
}

func (b *Bot) handleList(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	var entries []models.Entry
	b.db.Where("deleted_at IS NULL").Order("name ASC").Find(&entries)

	if len(entries) == 0 {
		b.replyText(msg.Chat.ID, "📭 No entries found in the database.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 <b>Total Entries: %d</b>\n\n", len(entries)))

	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("• <b>[#%d]</b> %s (%s) → %s\n", e.ID, html.EscapeString(e.Name), e.FirstLetter, html.EscapeString(e.URL)))
		if sb.Len() > 3500 {
			b.replyHTML(msg.Chat.ID, sb.String())
			sb.Reset()
		}
	}

	if sb.Len() > 0 {
		b.replyHTML(msg.Chat.ID, sb.String())
	}
}

func (b *Bot) handleSetChannel(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b> <code>/setchannel -1001234567890</code>\n\n<i>Note: Make sure to add the bot as an administrator in the channel!</i>")
		return
	}

	b.db.Save(&models.Setting{Key: "channel_id", Value: args})
	b.cfg.ChannelID = args

	b.replyHTML(msg.Chat.ID, fmt.Sprintf("✅ Channel set to: <code>%s</code>\n\n<i>Triggering initial sync...</i>", args))

	go func() {
		if err := b.syncSvc.SyncChannel(args); err != nil {
			b.replyText(msg.Chat.ID, fmt.Sprintf("⚠️ Channel set, but sync error: %v", err))
		}
	}()
}

func (b *Bot) handleAddAdmin(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b> <code>/addadmin &lt;TelegramID or @username&gt;</code>")
		return
	}

	cleanArg := strings.TrimPrefix(args, "@")

	var targetUser models.User
	var err error

	if id, parseErr := strconv.ParseInt(cleanArg, 10, 64); parseErr == nil {
		err = b.db.Where("telegram_id = ?", id).First(&targetUser).Error
	} else {
		err = b.db.Where("LOWER(username) = LOWER(?)", cleanArg).First(&targetUser).Error
	}

	if err != nil {
		// Create placeholder user record
		if id, parseErr := strconv.ParseInt(cleanArg, 10, 64); parseErr == nil {
			targetUser = models.User{TelegramID: id, IsAdmin: true}
			b.db.Create(&targetUser)
		} else {
			targetUser = models.User{Username: cleanArg, IsAdmin: true}
			b.db.Create(&targetUser)
		}
	} else {
		targetUser.IsAdmin = true
		b.db.Save(&targetUser)
	}

	b.replyHTML(msg.Chat.ID, fmt.Sprintf("👑 Granted Admin access to: <b>%s</b>", html.EscapeString(args)))
}

func (b *Bot) handleRemoveAdmin(msg *tgbotapi.Message, user *models.User) {
	if !user.IsSuperAdmin {
		b.replyText(msg.Chat.ID, "❌ Only Super Admins can remove other admins.")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b> <code>/removeadmin &lt;TelegramID or @username&gt;</code>")
		return
	}

	cleanArg := strings.TrimPrefix(args, "@")
	var targetUser models.User

	if id, err := strconv.ParseInt(cleanArg, 10, 64); err == nil {
		b.db.Where("telegram_id = ?", id).First(&targetUser)
	} else {
		b.db.Where("LOWER(username) = LOWER(?)", cleanArg).First(&targetUser)
	}

	if targetUser.ID == 0 {
		b.replyText(msg.Chat.ID, "❌ User not found.")
		return
	}

	if targetUser.IsSuperAdmin {
		b.replyText(msg.Chat.ID, "❌ Cannot remove Super Admin.")
		return
	}

	targetUser.IsAdmin = false
	b.db.Save(&targetUser)

	b.replyHTML(msg.Chat.ID, fmt.Sprintf("🚫 Revoked Admin access for: <b>%s</b>", html.EscapeString(args)))
}

func (b *Bot) handleListAdmins(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	var admins []models.User
	b.db.Where("is_admin = ?", true).Find(&admins)

	var sb strings.Builder
	sb.WriteString("👑 <b>Bot Administrators:</b>\n\n")

	for _, a := range admins {
		role := "Admin"
		if a.IsSuperAdmin {
			role = "Super Admin"
		}
		identifier := fmt.Sprintf("ID: %d", a.TelegramID)
		if a.Username != "" {
			identifier = "@" + a.Username
		}
		sb.WriteString(fmt.Sprintf("• <b>%s</b> (%s) - %s\n", html.EscapeString(a.FirstName), identifier, role))
	}

	b.replyHTML(msg.Chat.ID, sb.String())
}

func (b *Bot) handleAddFooter(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	text := strings.TrimSpace(msg.CommandArguments())

	// If no inline argument, check if the command is a reply to another message
	if text == "" && msg.ReplyToMessage != nil {
		if msg.ReplyToMessage.Text != "" {
			text = msg.ReplyToMessage.Text
		} else if msg.ReplyToMessage.Caption != "" {
			text = msg.ReplyToMessage.Caption
		}
	}

	if text == "" {
		b.replyHTML(msg.Chat.ID, "⚠️ <b>Usage:</b>\n• <code>/addfooter Your footer text here</code>\n• Or reply to any message with <code>/addfooter</code>")
		return
	}

	var count int64
	b.db.Model(&models.FooterMessage{}).Count(&count)

	footer := models.FooterMessage{
		Content:    text,
		OrderIndex: int(count),
	}
	b.db.Create(&footer)

	b.replyHTML(msg.Chat.ID, "📌 <b>Footer message added!</b>\n\n<i>Syncing channel and re-positioning footers at bottom...</i>")

	go func() {
		if err := b.syncSvc.SyncChannel(b.cfg.ChannelID); err != nil {
			log.Printf("Sync error after adding footer: %v", err)
		}
	}()
}

func (b *Bot) handleClearFooters(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	b.db.Exec("DELETE FROM footer_messages")
	b.replyText(msg.Chat.ID, "🧹 All footer messages cleared. Syncing channel...")

	go func() {
		if err := b.syncSvc.SyncChannel(b.cfg.ChannelID); err != nil {
			log.Printf("Sync error after clearing footers: %v", err)
		}
	}()
}

func (b *Bot) handleSync(msg *tgbotapi.Message, user *models.User) {
	if !user.IsAdmin {
		b.replyText(msg.Chat.ID, "❌ Permission denied.")
		return
	}

	b.replyText(msg.Chat.ID, "🔄 Starting channel synchronization...")

	err := b.syncSvc.SyncChannel(b.cfg.ChannelID)
	if err != nil {
		b.replyHTML(msg.Chat.ID, fmt.Sprintf("❌ <b>Sync Failed:</b> %v", html.EscapeString(err.Error())))
	} else {
		b.replyHTML(msg.Chat.ID, "✅ <b>Channel Synchronization Completed!</b>")
	}
}

func (b *Bot) replyText(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = b.api.Send(msg)
}

func (b *Bot) replyHTML(chatID int64, htmlText string) {
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, _ = b.api.Send(msg)
}
