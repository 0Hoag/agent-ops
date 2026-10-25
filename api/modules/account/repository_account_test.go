package account

import (
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func accountRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(gdb), mock
}

type bcryptHashArgument struct{ password string }

func (argument bcryptHashArgument) Match(value driver.Value) bool {
	hash, ok := value.(string)
	return ok && hash != argument.password && bcrypt.CompareHashAndPassword([]byte(hash), []byte(argument.password)) == nil
}

func TestCreateBindsHashedPasswordAndEmail(t *testing.T) {
	repository, mock := accountRepository(t)
	password := "secret password"
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "accounts"`)).WithArgs(sqlmock.AnyArg(), "person@example.com", bcryptHashArgument{password}, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	item, err := repository.Create(context.Background(), " person@example.com ", password)
	require.NoError(t, err)
	require.Equal(t, "person@example.com", item.Email)
	require.NotContains(t, item.PasswordHash, password)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateRejectsDuplicateIdentity(t *testing.T) {
	repository, mock := accountRepository(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "accounts"`)).WithArgs(sqlmock.AnyArg(), "person@example.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnError(errorsDuplicate())
	mock.ExpectRollback()
	_, err := repository.Create(context.Background(), "person@example.com", "password")
	require.ErrorContains(t, err, "create account")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateCredentialsFetchesBoundIdentityAndComparesPassword(t *testing.T) {
	repository, mock := accountRepository(t)
	password := "secret password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	columns := []string{"id", "email", "password_hash", "created_at", "updated_at"}
	mock.ExpectQuery(`SELECT \* FROM "accounts" WHERE email = \$1 ORDER BY "accounts"\."id" LIMIT \$2`).
		WithArgs("person@example.com", 1).
		WillReturnRows(sqlmock.NewRows(columns).AddRow("11111111-1111-1111-1111-111111111111", "person@example.com", string(hash), time.Now(), time.Now()))
	require.NoError(t, repository.ValidateCredentials(context.Background(), " person@example.com ", password))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateCredentialsRejectsUnknownAccountAndWrongPassword(t *testing.T) {
	repository, mock := accountRepository(t)
	mock.ExpectQuery(`SELECT \* FROM "accounts" WHERE email = \$1 ORDER BY "accounts"\."id" LIMIT \$2`).
		WithArgs("missing@example.com", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "created_at", "updated_at"}))
	require.ErrorIs(t, repository.ValidateCredentials(context.Background(), "missing@example.com", "password"), ErrInvalidCredentials)

	hash, err := bcrypt.GenerateFromPassword([]byte("right"), bcrypt.MinCost)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT \* FROM "accounts" WHERE email = \$1 ORDER BY "accounts"\."id" LIMIT \$2`).
		WithArgs("person@example.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash", "created_at", "updated_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "person@example.com", string(hash), time.Now(), time.Now()))
	require.ErrorIs(t, repository.ValidateCredentials(context.Background(), "person@example.com", "wrong"), ErrInvalidCredentials)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestValidateCredentialsRejectsMalformedRequiredInputBeforeSQL(t *testing.T) {
	repository, mock := accountRepository(t)
	require.ErrorIs(t, repository.ValidateCredentials(context.Background(), " ", "password"), ErrInvalidCredentials)
	require.ErrorIs(t, repository.ValidateCredentials(context.Background(), "person@example.com", ""), ErrInvalidCredentials)
	require.NoError(t, mock.ExpectationsWereMet())
}

func errorsDuplicate() error {
	return fmt.Errorf("duplicate key value violates unique constraint accounts_email_unique")
}
