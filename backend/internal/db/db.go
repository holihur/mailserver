package db

import (
	"mailserver/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(path string) (*gorm.DB, error) {
	g, err := gorm.Open(sqlite.Open(path+"?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 省内存少日志
	})
	if err != nil {
		return nil, err
	}
	sqlDB, _ := g.DB()
	sqlDB.SetMaxOpenConns(1) // SQLite + 低内存关键
	if err := g.AutoMigrate(&model.User{}, &model.Mail{}, &model.Domain{}, &model.DnsRecord{}); err != nil {
		return nil, err
	}
	return g, nil
}
