package auth

import (
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSessionRejectsInvalidAndExpiredTokens(t *testing.T) {
	service := NewService(nil, "secret", time.Hour)
	tests := []struct {
		name   string
		method jwt.SigningMethod
		claims jwt.MapClaims
		secret string
	}{
		{"malformed", nil, nil, ""},
		{"expired", jwt.SigningMethodHS256, jwt.MapClaims{"sub": "id", "email": "a@example.com", "exp": time.Now().Add(-time.Minute).Unix()}, "secret"},
		{"wrong signature", jwt.SigningMethodHS256, jwt.MapClaims{"sub": "id", "email": "a@example.com", "exp": time.Now().Add(time.Hour).Unix()}, "wrong"},
		{"unsupported algorithm", jwt.SigningMethodNone, jwt.MapClaims{"sub": "id", "email": "a@example.com", "exp": time.Now().Add(time.Hour).Unix()}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := "not-a-token"
			if test.method != nil {
				token := jwt.NewWithClaims(test.method, test.claims)
				key := interface{}([]byte(test.secret))
				if test.method == jwt.SigningMethodNone {
					key = jwt.UnsafeAllowNoneSignatureType
				}
				signed, err := token.SignedString(key)
				require.NoError(t, err)
				value = signed
			}
			_, err := service.Session(value)
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

func TestSessionRejectsTokenWithoutExpiration(t *testing.T) {
	service := NewService(nil, "secret", time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "account-id", "email": "person@example.com"})
	value, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	_, err = service.Session(value)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestSessionIdentityComesFromValidatedClaims(t *testing.T) {
	service := NewService(nil, "secret", time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "account-id", "email": "person@example.com", "exp": time.Now().Add(time.Hour).Unix()})
	value, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	session, err := service.Session(value)
	require.NoError(t, err)
	require.Equal(t, &SessionResponse{AccountID: "account-id", Email: "person@example.com"}, session)
}

func TestSessionRejectsExpiredAtBoundary(t *testing.T) {
	service := NewService(nil, "secret", time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "account-id", "email": "person@example.com", "exp": time.Now().Add(-time.Second).Unix(),
	})
	value, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	_, err = service.Session(value)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestHasMembershipChecksCurrentDatabaseMembership(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)
	service := NewService(account.NewRepository(db), "secret", time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).
		WithArgs("org-a", "acct-a").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	member, err := service.HasMembership(t.Context(), "acct-a", "org-a")
	require.NoError(t, err)
	require.False(t, member)
	require.NoError(t, mock.ExpectationsWereMet())
}
