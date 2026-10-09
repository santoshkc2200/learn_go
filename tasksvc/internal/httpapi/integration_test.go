package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/tasksvc/internal/store"
	"example.com/tasksvc/internal/tasks"
)

func TestSQLiteHTTPIntegration(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := httptest.NewServer(New(store.New(db), nil))
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Post(server.URL+"/tasks", "application/json", strings.NewReader(`{"title":"  real HTTP and SQL  "}`))
	if err != nil {
		t.Fatal(err)
	}
	var created tasks.Task
	err = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusCreated || created.Title != "real HTTP and SQL" || created.ID <= 0 || created.Status != "pending" || created.CreatedAt.IsZero() {
		t.Fatalf("POST=%+v status=%d error=%v", created, response.StatusCode, err)
	}
	response, err = client.Get(server.URL + response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var fetched tasks.Task
	err = json.NewDecoder(response.Body).Decode(&fetched)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || fetched != created {
		t.Fatalf("GET=%+v error=%v", fetched, err)
	}
	response, err = client.Get(server.URL + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Tasks []tasks.Task `json:"tasks"`
	}
	err = json.NewDecoder(response.Body).Decode(&listed)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(listed.Tasks) != 1 || listed.Tasks[0] != created {
		t.Fatalf("list=%+v error=%v", listed, err)
	}
}
