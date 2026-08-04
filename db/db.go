package db

import (
	"fmt"
	"log"

	"indexia/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(dbPath string) (*gorm.DB, error) {
	database, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	// Auto-migrate tables
	err = database.AutoMigrate(
		&models.User{},
		&models.Entry{},
		&models.ChannelMessage{},
		&models.FooterMessage{},
		&models.Setting{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}

	log.Println("Database initialized successfully at:", dbPath)
	return database, nil
}
