package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.DatabaseDriver))
	dsn := strings.TrimSpace(cfg.DatabaseDSN)
	if driver == "" {
		driver = "sqlite"
	}
	if dsn == "" {
		dsn = "./data/token-account-automation.db"
	}
	if driver == "sqlite" {
		if err := os.MkdirAll(filepath.Dir(dsn), 0755); err != nil && filepath.Dir(dsn) != "." {
			return nil, err
		}
		return gorm.Open(sqlite.Open(dsn), gormConfig())
	}
	if driver == "mysql" {
		return gorm.Open(mysql.Open(dsn), gormConfig())
	}
	if driver == "postgres" || driver == "postgresql" {
		return gorm.Open(postgres.Open(dsn), gormConfig())
	}
	return nil, fmt.Errorf("unsupported database driver: %s", driver)
}

func gormConfig() *gorm.Config {
	return &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
	}
}

func Migrate(database *gorm.DB) error {
	return database.AutoMigrate(
		&model.Job{},
		&model.Attempt{},
		&model.JobEvent{},
		&model.Target{},
		&model.TargetBinding{},
		&model.Secret{},
		&model.JobSecretRef{},
	)
}
