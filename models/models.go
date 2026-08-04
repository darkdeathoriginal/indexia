package models

import (
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TelegramID   int64     `gorm:"uniqueIndex;not null" json:"telegram_id"`
	Username     string    `gorm:"index" json:"username"`
	FirstName    string    `json:"first_name"`
	IsAdmin      bool      `gorm:"default:false" json:"is_admin"`
	IsSuperAdmin bool      `gorm:"default:false" json:"is_super_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Entry struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"not null;index" json:"name"`
	URL         string         `gorm:"not null" json:"url"`
	FirstLetter string         `gorm:"not null;index" json:"first_letter"`
	AddedByID   int64          `json:"added_by_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func GetFirstLetter(name string) string {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return "#"
	}
	r := []rune(trimmed)[0]
	rUpper := unicode.ToUpper(r)
	if rUpper >= 'A' && rUpper <= 'Z' {
		return string(rUpper)
	}
	return "#"
}

type ChannelMessage struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ChannelID   int64  `gorm:"not null;index" json:"channel_id"`
	MessageID   int    `gorm:"not null" json:"message_id"`
	MessageType string `gorm:"not null" json:"message_type"` // "alphabet" or "footer"
	Alphabet    string `json:"alphabet"`                    // "A", "B", etc.
	Part        int    `json:"part"`                        // 1, 2...
	OrderIndex  int    `gorm:"not null;index" json:"order_index"`
	ContentHash string `json:"content_hash"`
}

type FooterMessage struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Content       string    `gorm:"not null" json:"content"`
	OrderIndex    int       `gorm:"not null" json:"order_index"`
	TelegramMsgID int       `gorm:"default:0" json:"telegram_msg_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null" json:"value"`
}
