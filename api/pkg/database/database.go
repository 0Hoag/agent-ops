package database

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Pinger interface {
	PingContext(context.Context) error
}

func Open(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return db, nil
}

func Close(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql database: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

var _ Pinger = (*sql.DB)(nil)
var _ Pinger = pingAdapter{}

type pingAdapter struct{ db *gorm.DB }

func (p pingAdapter) PingContext(ctx context.Context) error { return ping(ctx, p.db) }

func AsPinger(db *gorm.DB) Pinger { return pingAdapter{db: db} }
