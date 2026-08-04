package bot

import (
	"log"

	"indexia/config"
	"indexia/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

type Bot struct {
	cfg     *config.Config
	db      *gorm.DB
	api     *tgbotapi.BotAPI
	syncSvc *service.SyncService
}

func NewBot(cfg *config.Config, db *gorm.DB) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	syncSvc := service.NewSyncService(db, api)

	return &Bot{
		cfg:     cfg,
		db:      db,
		api:     api,
		syncSvc: syncSvc,
	}, nil
}

func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go b.handleUpdate(update)
	}
}
