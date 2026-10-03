package db

import (
	"errors"
	"strings"

	"mailserver/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开数据库。driver: sqlite | postgres。dsn 为 postgres 连接串。
func Open(driver, path, dsn string) (*gorm.DB, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	var dial gorm.Dialector
	switch driver {
	case "postgres", "postgresql", "pg":
		if dsn == "" {
			return nil, errors.New("postgres 模式需要 DATABASE_URL")
		}
		dial = postgres.Open(dsn)
	case "", "sqlite", "sqlite3":
		dial = sqlite.Open(path + "?cache=shared")
	default:
		return nil, errors.New("不支持的数据库类型: " + driver)
	}

	g, err := gorm.Open(dial, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 省内存少日志
	})
	if err != nil {
		return nil, err
	}
	if sqlDB, err := g.DB(); err == nil && driver != "postgres" && driver != "postgresql" && driver != "pg" {
		sqlDB.SetMaxOpenConns(1) // SQLite + 低内存关键
	}
	if err := g.AutoMigrate(&model.User{}, &model.Mail{}, &model.Domain{}, &model.DnsRecord{}, &model.DnsProvider{}, &model.AcmeConfig{}, &model.Setting{}); err != nil {
		return nil, err
	}
	return g, nil
}
