package account

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrAccountNotFound = errors.New("account not found")

type Repository struct{ db *gorm.DB }

var dummyPasswordHash = func() string {
	hash, err := HashPassword("account credential timing sentinel")
	if err != nil {
		panic(fmt.Errorf("create dummy account password hash: %w", err))
	}
	return hash
}()

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Create(ctx context.Context, email, password string) (*Account, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return nil, errors.New("email and password are required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash account password: %w", err)
	}
	item := &Account{ID: uuid.NewString(), Email: email, PasswordHash: hash}
	if err := repository.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return item, nil
}

func (repository *Repository) GetByEmail(ctx context.Context, email string) (*Account, error) {
	var item Account
	if err := repository.db.WithContext(ctx).Where("email = ?", strings.TrimSpace(email)).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &item, nil
}

func (repository *Repository) ValidateCredentials(ctx context.Context, email, password string) error {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return ErrInvalidCredentials
	}
	var item Account
	if err := repository.db.WithContext(ctx).Where("email = ?", email).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = bcryptCompare(password, dummyPasswordHash)
			return ErrInvalidCredentials
		}
		return fmt.Errorf("get account credentials: %w", err)
	}
	if bcryptCompare(password, item.PasswordHash) != nil {
		return ErrInvalidCredentials
	}
	return nil
}
