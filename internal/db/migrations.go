package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RunMigrations(db *sql.DB, dir string) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}

	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(db)
	if err != nil {
		return err
	}

	for _, file := range files {
		alreadyApplied := applied[file]
		if alreadyApplied {
			continue
		}

		contents, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		upSQL := extractUpSQL(string(contents))
		if strings.TrimSpace(upSQL) == "" {
			continue
		}

		if _, err := db.Exec(upSQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}

		if !alreadyApplied {
			if _, err := db.Exec("insert into schema_migrations (filename) values ($1)", file); err != nil {
				return fmt.Errorf("record migration %s: %w", file, err)
			}
		}
	}

	return nil
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists schema_migrations (
    filename text primary key,
    applied_at timestamptz not null default now()
  )`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func migrationFiles(dir string) ([]string, error) {
	entries := []string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".sql") {
			entries = append(entries, d.Name())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk migrations: %w", err)
	}
	sort.Strings(entries)
	return entries, nil
}

func appliedMigrations(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("select filename from schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("scan migration: %w", err)
		}
		applied[filename] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows migrations: %w", err)
	}
	return applied, nil
}

func extractUpSQL(contents string) string {
	parts := strings.Split(contents, "-- +migrate Down")
	up := parts[0]
	up = strings.ReplaceAll(up, "-- +migrate Up", "")
	return up
}
