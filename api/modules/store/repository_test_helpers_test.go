package store

import (
	"database/sql"
	"database/sql/driver"
	"github.com/DATA-DOG/go-sqlmock"
)

func sqlmockResult() sql.Result { return sqlmock.NewResult(1, 1) }
func rows(column string, value any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{column}).AddRow(value)
}
func row(columns ...any) *sqlmock.Rows {
	names := make([]string, 0, len(columns)/2)
	values := make([]driver.Value, 0, len(columns)/2)
	for i := 0; i < len(columns); i += 2 {
		names = append(names, columns[i].(string))
		values = append(values, columns[i+1])
	}
	return sqlmock.NewRows(names).AddRow(values...)
}
