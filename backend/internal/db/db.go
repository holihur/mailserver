package db

import (
	"errors"
	"strings"

	"mailserver/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开 PostgreSQL（唯一数据库后端，纯 Go，无 CGO）。
func Open(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("需要 DATABASE_URL（PostgreSQL 连接串），例如 postgres://user:pass@host:5432/mailserver?sslmode=disable")
	}
	g, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 少日志
	})
	if err != nil {
		return nil, err
	}
	if err := g.AutoMigrate(&model.User{}, &model.Mail{}, &model.Domain{}, &model.DnsRecord{}, &model.DnsProvider{}, &model.AcmeConfig{}, &model.Setting{}, &model.MailToken{}, &model.MailRule{}, &model.Contact{}, &model.MailRoute{}, &model.MailAlias{}); err != nil {
		return nil, err
	}
	return g, nil
}
