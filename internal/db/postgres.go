package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

func NewPostgres(dsn string) (*sql.DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	conn.SetMaxOpenConns(20)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(30 * time.Minute)

	// retry a few times — useful when the postgres container is still
	// coming up when this service starts under docker-compose.
	var pingErr error
	for i := 0; i < 10; i++ {
		if pingErr = conn.Ping(); pingErr == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		return nil, fmt.Errorf("ping postgres: %w", pingErr)
	}

	if err := migrate(conn); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return conn, nil
}

func migrate(conn *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS ambulances (
		id             UUID PRIMARY KEY,
		driver_name    TEXT NOT NULL,
		vehicle_number TEXT NOT NULL UNIQUE,
		phone          TEXT NOT NULL,
		status         TEXT NOT NULL DEFAULT 'available',
		created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
	);
	`
	_, err := conn.Exec(schema)
	return err
}
