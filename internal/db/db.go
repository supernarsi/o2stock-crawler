package db

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	// ErrNoRows 表示没有找到记录
	ErrNoRows = errors.New("no rows found")
)

// DB is a thin wrapper over *gorm.DB to allow extension later.
type DB struct {
	*gorm.DB
}

// Open opens a new DB connection pool.
func Open(cfg *Config) (*DB, error) {
	logLevel := logger.Silent
	if cfg.Debug || isDev() {
		logLevel = logger.Info
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &DB{DB: db}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// isDev 启发式判断是否是 go run 执行 (通常在临时目录)
func isDev() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// go run 的二进制文件通常在临时目录中
	tempDir := os.TempDir()
	return strings.HasPrefix(exe, tempDir)
}
