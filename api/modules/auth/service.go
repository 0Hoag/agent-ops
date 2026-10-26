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
func (service *Service) SelectOrganization(ctx context.Context, token, organizationID string) (string, error) {
	session, err := service.Session(token)
	if err != nil {
		return "", err
	}
	var membership struct{ OrganizationID string }
	if err := service.accounts.DB().Table("organization_memberships").Where("organization_id = ? AND account_id = ?", organizationID, session.AccountID).First(&membership).Error; err != nil {
		return "", ErrInvalidToken
	}
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": session.AccountID, "email": session.Email, "organization_id": organizationID, "iat": now.Unix(), "exp": now.Add(service.expiration).Unix()})
	return t.SignedString(service.secret)
}

// HasMembership confirms that a selected organization is still assigned to the account.
// The signed selection token is not a durable authorization grant: membership changes
// must take effect before the token expires.
func (service *Service) HasMembership(ctx context.Context, accountID, organizationID string) (bool, error) {
	var count int64
	err := service.accounts.DB().WithContext(ctx).Table("organization_memberships").Where("organization_id = ? AND account_id = ?", organizationID, accountID).Count(&count).Error
	return count == 1, err
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
	organizationID, _ := claims["organization_id"].(string)
	return &SessionResponse{AccountID: sub, Email: email, OrganizationID: organizationID}, nil
}
