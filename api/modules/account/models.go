package account

import "time"

type Account struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	Email        string `gorm:"not null;uniqueIndex"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (Account) TableName() string { return "accounts" }
