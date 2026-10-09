// Package httpapi owns the transport boundary, including public error messages.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"example.com/tasksvc/internal/tasks"
)

// TaskStore belongs to its consumer. Store satisfies it without importing HTTP.
type TaskStore interface {
	Create(context.Context, string) (tasks.Task, error)
	Get(context.Context, int64) (tasks.Task, error)
	List(context.Context) ([]tasks.Task, error)
}

type api struct {
	store  TaskStore
	logger *slog.Logger
}

func New(store TaskStore, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	a := &api{store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", a.create)
	mux.HandleFunc("GET /tasks/{id}", a.get)
	mux.HandleFunc("GET /tasks", a.list)
	return mux
}

func (a *api) create(w http.ResponseWriter, r *http.Request) {
	title, status, err := decodeCreate(w, r)
	if err != nil {
		a.writeError(w, status, err.Error())
		return
	}
	task, err := a.store.Create(r.Context(), title)
	if err != nil {
		a.storageError(w, r, err)
		return
	}
	w.Header().Set("Location", "/tasks/"+strconv.FormatInt(task.ID, 10))
	a.writeJSON(w, http.StatusCreated, task)
}

func (a *api) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		a.writeError(w, 400, "invalid task ID")
		return
	}
	task, err := a.store.Get(r.Context(), id)
	if err != nil {
		a.storageError(w, r, err)
		return
	}
	a.writeJSON(w, 200, task)
}

func (a *api) list(w http.ResponseWriter, r *http.Request) {
	// No query options exist yet; rejecting raw text also catches malformed pairs.
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		a.writeError(w, 400, "query parameters are not supported")
		return
	}
	list, err := a.store.List(r.Context())
	if err != nil {
		a.storageError(w, r, err)
		return
	}
	if list == nil {
		list = make([]tasks.Task, 0)
	}
	a.writeJSON(w, 200, struct {
		Tasks []tasks.Task `json:"tasks"`
	}{list})
}

func (a *api) storageError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		a.writeError(w, 404, "task not found")
	case errors.Is(err, tasks.ErrInvalidTitle):
		a.writeError(w, 400, "invalid task title")
	default:
		a.logger.Error("storage operation failed", "method", r.Method, "path", r.URL.Path, "error", err)
		a.writeError(w, 500, "internal server error")
	}
}
