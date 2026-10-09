package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/tasksvc/internal/tasks"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestCreateGetList(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	task, err := s.Create(ctx, "  learn SQL\t")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID <= 0 || task.Title != "learn SQL" || task.Status != "pending" || task.CreatedAt.IsZero() || task.CreatedAt.Location() != time.UTC {
		t.Fatalf("unexpected task: %+v", task)
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil || got != task {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, task)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0] != task {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if _, err := s.Create(ctx, " "); !errors.Is(err, tasks.ErrInvalidTitle) {
		t.Fatalf("invalid title: %v", err)
	}
}

func TestEmptyList(t *testing.T) {
	got, err := New(testDB(t)).List(context.Background())
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("List = %v, %v", got, err)
	}
}

func TestNotFound(t *testing.T) {
	_, err := New(testDB(t)).Get(context.Background(), 99)
	if !errors.Is(err, tasks.ErrNotFound) {
		t.Fatalf("Get error = %v", err)
	}
}

func TestLiteralTitle(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	title := "Robert'); DROP TABLE tasks;--"
	task, err := s.Create(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil || got.Title != title {
		t.Fatalf("literal title = %+v, %v", got, err)
	}
	if _, err := s.Create(ctx, "table survived"); err != nil {
		t.Fatal(err)
	}
}

func TestRestartPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	ctx := context.Background()
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	task, err := New(db).Create(ctx, "persistent")
	closeErr := db.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("create/close: %v / %v", err, closeErr)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := New(db).Get(ctx, task.ID)
	if err != nil || got != task {
		t.Fatalf("reopened task = %+v, %v", got, err)
	}
}

func TestRepeatedInitialization(t *testing.T) {
	db := testDB(t)
	for range 2 {
		if err := initialize(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIncompatibleSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE tasks (id INTEGER PRIMARY KEY); INSERT INTO tasks VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if opened, err := Open(context.Background(), path); err == nil {
		opened.Close()
		t.Fatal("accepted incompatible schema")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err := db.QueryRow("SELECT id FROM tasks").Scan(&id); err != nil || id != 7 {
		t.Fatalf("existing data changed: %d, %v", id, err)
	}
}

func TestListLimit(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	for range 105 {
		if _, err := s.Create(ctx, "task"); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 100 {
		t.Fatalf("len=%d, error=%v", len(list), err)
	}
	for i, task := range list {
		if task.ID != int64(i+1) {
			t.Fatalf("order at %d: %d", i, task.ID)
		}
	}
}

func TestDatabasePathCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks with # spaces.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := New(db).Create(context.Background(), "path works"); err != nil {
		t.Fatal(err)
	}
}

func TestDSNPathEncoder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a #?.db")
	dsn, err := databaseDSN(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	decoded := u.Path
	if filepath.VolumeName(path) != "" {
		decoded = strings.TrimPrefix(decoded, "/")
	}
	if u.Host != "" || u.Fragment != "" || len(u.Query()) != 1 || len(u.Query()["_pragma"]) != 2 || filepath.FromSlash(decoded) != path {
		t.Fatalf("path/config mixed: %s", dsn)
	}
}

func TestCanceledQuery(t *testing.T) {
	s := New(testDB(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Get(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestCanceledPoolWait(t *testing.T) {
	db := testDB(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := New(db).Get(ctx, 1); result <- err }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for db.Stats().WaitCount == 0 {
		select {
		case <-deadline.C:
			t.Fatal("query did not wait for occupied connection")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock pool wait")
	}
}
