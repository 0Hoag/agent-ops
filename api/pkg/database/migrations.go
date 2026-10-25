package database

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	appmigrations "github.com/0Hoag/agent-ops/api/migrations"
	"gorm.io/gorm"
)

type migration struct {
	Version int
	Name    string
	SQL     string
}

func RunMigrations(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`).Error; err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, item := range migrations {
		var applied int64
		if err := db.WithContext(ctx).Table("schema_migrations").Where("version = ?", item.Version).Count(&applied).Error; err != nil {
			return fmt.Errorf("check migration %03d: %w", item.Version, err)
		}
		if applied > 0 {
			continue
		}
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(item.SQL).Error; err != nil {
				return fmt.Errorf("apply migration %03d: %w", item.Version, err)
			}
			return tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", item.Version, item.Name).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	var migrations []migration
	err := fs.WalkDir(appmigrations.FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".sql") {
			return walkErr
		}
		base := strings.TrimSuffix(strings.TrimPrefix(path, "./"), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return fmt.Errorf("migration %q must be NNN_name.sql", path)
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return fmt.Errorf("migration %q has invalid version", path)
		}
		contents, err := fs.ReadFile(appmigrations.FS, path)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", path, err)
		}
		name := parts[1]
		name = strings.TrimSuffix(name, "_table")
		migrations = append(migrations, migration{Version: version, Name: name, SQL: string(contents)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version == migrations[i].Version {
			return nil, fmt.Errorf("duplicate migration version %03d", migrations[i].Version)
		}
	}
	return migrations, nil
}
