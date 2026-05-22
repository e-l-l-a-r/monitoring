package repository

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type SqlStorage struct {
	MemStorage
	db *sql.DB
}

func NewSqlStorage(connectionStr string) (res *SqlStorage, err error) {
	db, err := sql.Open("pgx", connectionStr)
	if err != nil {
		return nil, err
	}

	return &SqlStorage{
		MemStorage: *NewMemStorage(0, "/dev/null"),
		db:         db,
	}, nil
}

func (sql *SqlStorage) Close() {
	sql.db.Close()
}

func (sql *SqlStorage) Ping() error {
	return sql.db.Ping()
}
