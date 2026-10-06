package db

import (
	"errors"
	"strings"
	"time"

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
	// 连接池上限：避免多包测试/多实例把 PG 连接打满（max_connections）。
	if sqlDB, err := g.DB(); err == nil {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(2)
		sqlDB.SetConnMaxIdleTime(2 * time.Minute)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	if err := g.AutoMigrate(&model.User{}, &model.Mail{}, &model.Domain{}, &model.DnsRecord{}, &model.DnsProvider{}, &model.AcmeConfig{}, &model.Setting{}, &model.MailToken{}, &model.MailRule{}, &model.Contact{}, &model.MailRoute{}, &model.MailAlias{}, &model.ExternalAccount{}, &model.MailFolder{}, &model.SieveScript{}, &model.ScheduledMail{}, &model.AuditLog{}, &model.LoginEvent{}, &model.Session{}, &model.ExternalSync{}, &model.AIProvider{}); err != nil {
		return nil, err
	}
	// 全文/子串检索加速：pg_trgm + GIN 索引（subject/from/to/body 的 ILIKE）。
	// 扩展不可用时忽略，仅退化为顺序扫描，功能不受影响。
	_ = g.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_mails_subject_trgm ON mails USING gin (subject gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_mails_body_trgm ON mails USING gin (body gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_mails_from_trgm ON mails USING gin ("from" gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_mails_to_trgm ON mails USING gin ("to" gin_trgm_ops)`,
	} {
		_ = g.Exec(idx).Error
	}
	return g, nil
}
