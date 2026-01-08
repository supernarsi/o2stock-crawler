package db

import (
	"database/sql"
	"errors"

	_ "github.com/go-sql-driver/mysql"
)

var (
	// ErrNoRows 表示没有找到记录
	ErrNoRows = errors.New("no rows found")
)

// DB is a thin wrapper over *sql.DB to allow extension later.
type DB struct {
	*sql.DB
}

// Open opens a new DB connection pool.
func Open(cfg *Config) (*DB, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, err
	}
	// Basic sanity check.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{DB: db}, nil
}
