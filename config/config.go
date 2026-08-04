package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken  string
	ChannelID string
	DBPath    string
}

func LoadConfig() (*Config, error) {
	// Load .env file if present, ignore error if missing
	_ = godotenv.Load()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "indexia.db"
	}

	return &Config{
		BotToken:  os.Getenv("BOT_TOKEN"),
		ChannelID: os.Getenv("CHANNEL_ID"),
		DBPath:    dbPath,
	}, nil
}
