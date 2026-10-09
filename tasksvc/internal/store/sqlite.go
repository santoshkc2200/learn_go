// Package store implements persistence; its SQL details never reach HTTP clients.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"example.com/tasksvc/internal/tasks"
	_ "modernc.org/sqlite" // registers database/sql's "sqlite" driver
)

//go:embed schema.sql
var schema string

func databaseDSN(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute database path: %w", err)
	}
	// Encode the filename first, so # and ? cannot become URI configuration.
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Open returns an initialized pool. Its successful caller owns Close.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn, err := databaseDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := initialize(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initialize(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema initialization: %w", err)
	}
	defer tx.Rollback() // harmless after Commit; essential on every early return
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	// Stay on tx: querying db here would wait forever for our sole connection.
	rows, err := tx.QueryContext(ctx, "SELECT id, title, status, created_at FROM tasks LIMIT 0")
	if err != nil {
		return fmt.Errorf("incompatible schema: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close schema check: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema initialization: %w", err)
	}
	return nil
}

// Store borrows the pool; it must not close a resource owned by the executable.
type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, title string) (tasks.Task, error) {
	title, err := tasks.NormalizeTitle(title)
	if err != nil {
		return tasks.Task{}, err
	}
	task := tasks.Task{Title: title, Status: "pending", CreatedAt: time.Now().UTC()}
	result, err := s.db.ExecContext(ctx, "INSERT INTO tasks (title, status, created_at) VALUES (?, ?, ?)", task.Title, task.Status, task.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return tasks.Task{}, fmt.Errorf("create task: %w", err)
	}
	task.ID, err = result.LastInsertId()
	if err != nil {
		return tasks.Task{}, fmt.Errorf("task insert ID: %w", err)
	}
	return task, nil
}

type scanner interface{ Scan(...any) error }

func scanTask(row scanner) (tasks.Task, error) {
	var task tasks.Task
	var timestamp string
	if err := row.Scan(&task.ID, &task.Title, &task.Status, &timestamp); err != nil {
		return tasks.Task{}, err
	}
	var err error
	task.CreatedAt, err = time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return tasks.Task{}, fmt.Errorf("parse stored timestamp: %w", err)
	}
	return task, nil
}

func (s *Store) Get(ctx context.Context, id int64) (tasks.Task, error) {
	task, err := scanTask(s.db.QueryRowContext(ctx, "SELECT id, title, status, created_at FROM tasks WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return tasks.Task{}, fmt.Errorf("get task %d: %w", id, tasks.ErrNotFound)
	}
	if err != nil {
		return tasks.Task{}, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (s *Store) List(ctx context.Context) ([]tasks.Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, title, status, created_at FROM tasks ORDER BY id ASC LIMIT 100")
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	list := make([]tasks.Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		list = append(list, task)
	}
	// Next returning false may mean exhaustion OR an interrupted query.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return list, nil
}
