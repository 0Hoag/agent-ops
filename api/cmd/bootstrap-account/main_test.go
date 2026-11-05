package main

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func mockDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, mock
}

func TestRunRejectsInvalidInputBeforeOpeningDatabase(t *testing.T) {
	for _, test := range []struct {
		name, email, password, want string
	}{
		{"missing email", "", "secret", "valid address"},
		{"invalid email", "not-an-email", "secret", "valid address"},
		{"empty password", "person@example.com", "\n", "must not be empty"},
		{"long password", "person@example.com", strings.Repeat("x", 73), "72-byte"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(context.Background(), []string{"--email", test.email}, strings.NewReader(test.password), &bytes.Buffer{}, &bytes.Buffer{}, func(string) (*gorm.DB, error) {
				t.Fatal("database must not be opened")
				return nil, nil
			})
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestRunCreatesAccountWithoutLeakingSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test.invalid/ignored")
	db, mock := mockDatabase(t)
	mock.ExpectPing()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "accounts"`)).
		WithArgs(sqlmock.AnyArg(), "person@example.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	password := "secret-value"
	err := run(context.Background(), []string{"--email", "person@example.com"}, strings.NewReader(password+"\n"), stdout, stderr, func(string) (*gorm.DB, error) { return db, nil })
	require.NoError(t, err)
	require.Equal(t, "Account created for person@example.com.\n", stdout.String())
	require.NotContains(t, stdout.String(), password)
	require.NotContains(t, stderr.String(), password)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunRefusesDuplicateWithoutReplacingCredential(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test.invalid/ignored")
	db, mock := mockDatabase(t)
	mock.ExpectPing()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "accounts"`)).
		WithArgs(sqlmock.AnyArg(), "person@example.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New(`ERROR: duplicate key value violates unique constraint "uni_accounts_email"`))
	mock.ExpectRollback()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := run(context.Background(), []string{"--email", "person@example.com"}, strings.NewReader("replacement\n"), stdout, stderr, func(string) (*gorm.DB, error) { return db, nil })
	require.ErrorContains(t, err, "could not be created")
	require.Empty(t, stdout.String())
	require.NotContains(t, stderr.String(), "replacement")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunSanitizesDatabaseErrors(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test.invalid/ignored")
	db, mock := mockDatabase(t)
	mock.ExpectPing().WillReturnError(errors.New("postgres://user:password@db/secret"))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := run(context.Background(), []string{"--email", "person@example.com"}, strings.NewReader("secret\n"), stdout, stderr, func(string) (*gorm.DB, error) { return db, nil })
	require.ErrorContains(t, err, "database unavailable")
	require.NotContains(t, err.Error(), "password")
	require.NoError(t, mock.ExpectationsWereMet())
}
