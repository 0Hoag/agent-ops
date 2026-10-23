package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// UUID is a canonical UUID string. PostgreSQL enforces its UUID format.
type UUID string

// JSONB keeps persistence independent from any SCM provider package.
type JSONB []byte

func (value JSONB) Value() (driver.Value, error) {
	if len(value) == 0 {
		return []byte("{}"), nil
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("invalid JSONB payload")
	}
	return []byte(value), nil
}

func (value *JSONB) Scan(source any) error {
	switch data := source.(type) {
	case nil:
		*value = nil
	case []byte:
		*value = append((*value)[:0], data...)
	case string:
		*value = append((*value)[:0], data...)
	default:
		return fmt.Errorf("cannot scan %T as JSONB", source)
	}
	return nil
}

func (JSONB) GormDataType() string { return "jsonb" }

type Page struct {
	Page    int
	PerPage int
	Total   int64
}

func pageArgs(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return page, perPage
}
