package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const maxBcryptPasswordBytes = 72

var openDatabase = func(databaseURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, openDatabase); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, open func(string) (*gorm.DB, error)) error {
	flags := flag.NewFlagSet("bootstrap-account", flag.ContinueOnError)
	flags.SetOutput(stderr)
	email := flags.String("email", "", "account email address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	address := strings.TrimSpace(*email)
	if err := validateEmail(address); err != nil {
		return err
	}
	passwordBytes, err := io.ReadAll(stdin)
	if err != nil {
		return errors.New("read password: input unavailable")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(passwordBytes), "\n"), "\r")
	if password == "" {
		return errors.New("password must not be empty")
	}
	if len([]byte(password)) > maxBcryptPasswordBytes {
		return errors.New("password exceeds bcrypt's 72-byte limit")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := open(databaseURL)
	if err != nil {
		return errors.New("open database: database unavailable")
	}
	defer database.Close(db)
	if err := database.AsPinger(db).PingContext(ctx); err != nil {
		return errors.New("connect database: database unavailable")
	}
	_, err = account.NewRepository(db).Create(ctx, address, password)
	if err != nil {
		return errors.New("create account: account could not be created (it may already exist)")
	}
	if _, err := fmt.Fprintf(stdout, "Account created for %s.\n", address); err != nil {
		return errors.New("write result: output unavailable")
	}
	return nil
}

func validateEmail(address string) error {
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || !strings.Contains(address, "@") {
		return errors.New("email must be a valid address")
	}
	return nil
}
