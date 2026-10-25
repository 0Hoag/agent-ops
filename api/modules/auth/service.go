package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type Service struct {
	accounts   *account.Repository
	secret     []byte
	expiration time.Duration
}

func NewService(accounts *account.Repository, secret string, expiration time.Duration) *Service {
	if strings.TrimSpace(secret) == "" {
		panic("JWT signing key is required")
	}
	if expiration <= 0 {
		panic("JWT expiration must be positive")
	}
	return &Service{accounts: accounts, secret: []byte(secret), expiration: expiration}
}
func (service *Service) Login(ctx context.Context, email, password string) (string, error) {
	if err := service.accounts.ValidateCredentials(ctx, email, password); err != nil {
		return "", err
	}
	item, err := service.accounts.GetByEmail(ctx, email)
	if err != nil {
		return "", fmt.Errorf("get login account: %w", err)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": item.ID, "email": item.Email, "iat": now.Unix(), "exp": now.Add(service.expiration).Unix()})
	return token.SignedString(service.secret)
}
func (service *Service) Session(value string) (*SessionResponse, error) {
	token, err := jwt.Parse(value, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return service.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	sub, a := claims["sub"].(string)
	email, b := claims["email"].(string)
	if !a || !b || sub == "" || email == "" {
		return nil, ErrInvalidToken
	}
	return &SessionResponse{AccountID: sub, Email: email}, nil
}
