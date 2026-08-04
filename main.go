package main

import (
	"log"

	"indexia/bot"
	"indexia/config"
	"indexia/db"
)

func main() {
	log.Println("Starting Telegram Alphabetical Channel Directory Bot...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.BotToken == "" {
		log.Println("⚠️ WARNING: BOT_TOKEN is empty! Please set BOT_TOKEN in your environment or .env file.")
	}

	database, err := db.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if cfg.BotToken == "" {
		log.Fatalf("Cannot start bot without BOT_TOKEN. Exiting.")
	}

	telegramBot, err := bot.NewBot(cfg, database)
	if err != nil {
		log.Fatalf("Failed to create Telegram bot: %v", err)
	}

	log.Println("Bot initialized. Starting update listener...")
	telegramBot.Start()
}
