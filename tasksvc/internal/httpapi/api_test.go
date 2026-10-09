package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/tasksvc/internal/tasks"
)

// A controlled storage boundary injects failures without faking SQL behavior.
type controlledStore struct {
	err  error
	ctx  context.Context
	task tasks.Task
}

func (s *controlledStore) Create(ctx context.Context, title string) (tasks.Task, error) {
	s.ctx = ctx
	s.task = tasks.Task{ID: 7, Title: title, Status: "pending", CreatedAt: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}
	return s.task, s.err
}
func (s *controlledStore) Get(ctx context.Context, id int64) (tasks.Task, error) {
	s.ctx = ctx
	return s.task, s.err
}
func (s *controlledStore) List(ctx context.Context) ([]tasks.Task, error) {
	s.ctx = ctx
	return nil, s.err
}
func perform(h http.Handler, method, path, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body)
	}
}

func TestCreateTask(t *testing.T) {
	s := &controlledStore{}
	h := New(s, nil)
	r := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"title":"  learn HTTP  "}`))
	r.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(r.Context(), struct{}{}, "request")
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertStatus(t, w, 201)
	var got tasks.Task
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != s.task || got.Title != "learn HTTP" || s.ctx != ctx || w.Header().Get("Location") != "/tasks/7" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response=%+v headers=%v context=%v", got, w.Header(), s.ctx)
	}
}
func TestGetTask(t *testing.T) {
	s := &controlledStore{task: tasks.Task{ID: 7, Title: "known", Status: "pending"}}
	w := perform(New(s, nil), "GET", "/tasks/7", "", "")
	assertStatus(t, w, 200)
	var got tasks.Task
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != s.task || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("got=%+v", got)
	}
}
func TestListTasks(t *testing.T) {
	w := perform(New(&controlledStore{}, nil), "GET", "/tasks", "", "")
	assertStatus(t, w, 200)
	var got struct {
		Tasks []tasks.Task `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Tasks == nil || len(got.Tasks) != 0 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("body=%s", w.Body)
	}
}
func TestCreateInput(t *testing.T) {
	cases := []struct {
		name, body, media string
		status            int
		title             string
	}{
		{"absent media", `{"title":"x"}`, "", 415, ""},
		{"wrong media", `{"title":"x"}`, "text/plain", 415, ""},
		{"malformed media", `{"title":"x"}`, "application/json; broken", 415, ""},
		{"charset", `{"title":"x"}`, "application/json; charset=utf-8", 201, "x"},
		{"empty", "", "application/json", 400, ""},
		{"null", "null", "application/json", 400, ""},
		{"unknown", `{"title":"x","status":"done"}`, "application/json", 400, ""},
		{"trailing", `{"title":"x"} {}`, "application/json", 400, ""},
		{"type", `{"title":42}`, "application/json", 400, ""},
		{"array", `[]`, "application/json", 400, ""},
		{"missing", `{}`, "application/json", 400, ""},
		{"blank", `{"title":"   "}`, "application/json", 400, ""},
		{"title null", `{"title":null}`, "application/json", 400, ""},
		{"rune limit", fmt.Sprintf(`{"title":%q}`, strings.Repeat("界", 200)), "application/json", 201, strings.Repeat("界", 200)},
		{"too many runes", fmt.Sprintf(`{"title":%q}`, strings.Repeat("界", 201)), "application/json", 400, ""},
		{"duplicate", `{"title":"first","title":"last"}`, "application/json", 201, "last"},
		{"duplicate then null", `{"title":"first","title":null}`, "application/json", 400, ""},
		{"null then string", `{"title":null,"title":"last"}`, "application/json", 201, "last"},
		{"surrogate", `{"title":"\ud800"}`, "application/json", 201, "�"},
		{"raw invalid UTF8", "{\"title\":\"" + string([]byte{0xff}) + "\"}", "application/json", 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &controlledStore{}
			w := perform(New(s, nil), "POST", "/tasks", tc.body, tc.media)
			assertStatus(t, w, tc.status)
			if tc.status == 201 && s.task.Title != tc.title {
				t.Fatalf("title=%q want=%q", s.task.Title, tc.title)
			}
			if tc.status != 201 && s.ctx != nil {
				t.Fatal("invalid request reached storage")
			}
		})
	}
}
func TestBodyLimit(t *testing.T) {
	prefix := `{"title":"x"}`
	for _, size := range []int{16384, 16385} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := prefix + strings.Repeat(" ", size-len(prefix))
			w := perform(New(&controlledStore{}, nil), "POST", "/tasks", body, "application/json")
			want := 201
			if size > 16384 {
				want = 413
			}
			assertStatus(t, w, want)
		})
	}
}
func TestRouting(t *testing.T) {
	h := New(&controlledStore{}, nil)
	assertStatus(t, perform(h, "GET", "/unknown", "", ""), 404)
	w := perform(h, "PUT", "/tasks", "", "")
	assertStatus(t, w, 405)
	for _, method := range []string{"GET", "HEAD", "POST"} {
		if !strings.Contains(w.Header().Get("Allow"), method) {
			t.Fatalf("Allow=%q", w.Header().Get("Allow"))
		}
	}
}
func TestInvalidID(t *testing.T) {
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			assertStatus(t, perform(New(&controlledStore{}, nil), "GET", "/tasks/"+id, "", ""), 400)
		})
	}
}
func TestUnsupportedQuery(t *testing.T) {
	for _, query := range []string{"?limit=1", "?bad=%zz", "?x"} {
		t.Run(query, func(t *testing.T) {
			assertStatus(t, perform(New(&controlledStore{}, nil), "GET", "/tasks"+query, "", ""), 400)
		})
	}
}
func TestStorageFailureSanitized(t *testing.T) {
	var logs bytes.Buffer
	private := errors.New("private database path and secret")
	h := New(&controlledStore{err: private}, slog.New(slog.NewTextHandler(&logs, nil)))
	for _, request := range []struct{ method, path, body string }{{"GET", "/tasks/7", ""}, {"GET", "/tasks", ""}, {"POST", "/tasks", `{"title":"x"}`}} {
		w := perform(h, request.method, request.path, request.body, "application/json")
		assertStatus(t, w, 500)
		if strings.Contains(w.Body.String(), private.Error()) || !strings.Contains(w.Body.String(), "internal server error") {
			t.Fatalf("unsafe response: %s", w.Body)
		}
	}
	if !strings.Contains(logs.String(), private.Error()) {
		t.Fatalf("missing diagnostics: %s", &logs)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{fmt.Errorf("wrapped: %w", tasks.ErrNotFound), 404}, {tasks.ErrInvalidTitle, 400}} {
		assertStatus(t, perform(New(&controlledStore{err: tc.err}, nil), "GET", "/tasks/7", "", ""), tc.status)
	}
}

type failingWriter struct {
	header          http.Header
	writes, headers int
}

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) WriteHeader(int)           { w.headers++ }
func (w *failingWriter) Write([]byte) (int, error) { w.writes++; return 0, io.ErrClosedPipe }
func TestResponseWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	w := &failingWriter{header: make(http.Header)}
	New(&controlledStore{}, slog.New(slog.NewTextHandler(&logs, nil))).ServeHTTP(w, httptest.NewRequest("GET", "/tasks", nil))
	if w.writes != 1 || w.headers != 1 || !strings.Contains(logs.String(), io.ErrClosedPipe.Error()) {
		t.Fatalf("writes=%d headers=%d logs=%s", w.writes, w.headers, &logs)
	}
}
