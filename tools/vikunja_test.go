package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setupVikunjaServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"unauthorized"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/v1/tasks/all" && r.Method == "GET":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "title": "Buy groceries", "done": false, "priority": 2, "due_date": "2026-03-15T00:00:00Z"},
				{"id": 2, "title": "Write tests", "done": true, "priority": 0, "due_date": "0001-01-01T00:00:00Z"},
				{"id": 3, "title": "Deploy app", "done": false, "priority": 5, "due_date": "2026-04-01T00:00:00Z"},
			})

		case strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"id": 1, "title": "Buy groceries", "description": "Milk, eggs, bread",
				"done": false, "priority": 2, "due_date": "2026-03-15T00:00:00Z",
				"project_id": 1, "created": "2026-03-01T10:00:00Z", "updated": "2026-03-08T12:00:00Z",
				"labels":    []map[string]any{{"id": 1, "title": "errands"}},
				"assignees": []map[string]any{{"id": 1, "username": "testuser"}},
			})

		case strings.HasPrefix(r.URL.Path, "/api/v1/projects/") && r.Method == "PUT":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			json.NewEncoder(w).Encode(map[string]any{
				"id": 42, "title": body["title"],
			})

		case strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && r.Method == "POST":
			w.Write([]byte(`{"id":1,"title":"Updated"}`))

		case strings.HasPrefix(r.URL.Path, "/api/v1/tasks/") && r.Method == "DELETE":
			w.Write([]byte(`{}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func withVikunjaEnv(t *testing.T, serverURL string, fn func()) {
	t.Helper()
	origKey := os.Getenv("VIKUNJA_API_KEY")
	origURL := os.Getenv("VIKUNJA_URL")
	defer func() {
		setOrUnset("VIKUNJA_API_KEY", origKey)
		setOrUnset("VIKUNJA_URL", origURL)
	}()

	os.Setenv("VIKUNJA_API_KEY", "test-token")
	os.Setenv("VIKUNJA_URL", serverURL)
	fn()
}

func setOrUnset(key, val string) {
	if val != "" {
		os.Setenv(key, val)
	} else {
		os.Unsetenv(key)
	}
}

func TestVikunjaShouldInclude(t *testing.T) {
	origKey := os.Getenv("VIKUNJA_API_KEY")
	origURL := os.Getenv("VIKUNJA_URL")
	defer func() {
		setOrUnset("VIKUNJA_API_KEY", origKey)
		setOrUnset("VIKUNJA_URL", origURL)
	}()

	tool := ListVikunjaTasks()

	os.Unsetenv("VIKUNJA_API_KEY")
	os.Unsetenv("VIKUNJA_URL")
	if tool.ShouldInclude() {
		t.Error("ShouldInclude should be false when env vars are unset")
	}

	os.Setenv("VIKUNJA_API_KEY", "key")
	os.Unsetenv("VIKUNJA_URL")
	if tool.ShouldInclude() {
		t.Error("ShouldInclude should be false when only API key is set")
	}

	os.Unsetenv("VIKUNJA_API_KEY")
	os.Setenv("VIKUNJA_URL", "http://localhost")
	if tool.ShouldInclude() {
		t.Error("ShouldInclude should be false when only URL is set")
	}

	os.Setenv("VIKUNJA_API_KEY", "key")
	os.Setenv("VIKUNJA_URL", "http://localhost")
	if !tool.ShouldInclude() {
		t.Error("ShouldInclude should be true when both env vars are set")
	}
}

func TestListVikunjaTasks(t *testing.T) {
	server := setupVikunjaServer()
	defer server.Close()

	tool := ListVikunjaTasks()

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, "Buy groceries") {
			t.Error("expected result to contain 'Buy groceries'")
		}
		if !strings.Contains(result, "[x]") {
			t.Error("expected result to show done marker")
		}
		if !strings.Contains(result, "(priority: 2)") {
			t.Error("expected result to show priority")
		}
		if !strings.Contains(result, "(due: 2026-03-15)") {
			t.Error("expected result to show due date")
		}
	})
}

func TestGetVikunjaTask(t *testing.T) {
	server := setupVikunjaServer()
	defer server.Close()

	tool := GetVikunjaTask()

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{"id": 1})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, "Buy groceries") {
			t.Error("expected title")
		}
		if !strings.Contains(result, "Milk, eggs, bread") {
			t.Error("expected description")
		}
		if !strings.Contains(result, "errands") {
			t.Error("expected labels")
		}
		if !strings.Contains(result, "testuser") {
			t.Error("expected assignees")
		}
	})
}

func TestCreateVikunjaTask(t *testing.T) {
	server := setupVikunjaServer()
	defer server.Close()

	tool := CreateVikunjaTask()

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{
			"title":      "New task",
			"project_id": 1,
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, "Task created with ID: 42") {
			t.Errorf("unexpected result: %s", result)
		}
	})
}

func TestUpdateVikunjaTask(t *testing.T) {
	server := setupVikunjaServer()
	defer server.Close()

	tool := UpdateVikunjaTask()

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{
			"id":   1,
			"done": true,
		})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, "updated") {
			t.Errorf("unexpected result: %s", result)
		}
		if !strings.Contains(result, "done") {
			t.Errorf("expected 'done' in changed fields: %s", result)
		}
	})

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{"id": 1})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "No changes provided." {
			t.Errorf("expected no changes message, got: %s", result)
		}
	})
}

func TestDeleteVikunjaTask(t *testing.T) {
	server := setupVikunjaServer()
	defer server.Close()

	tool := DeleteVikunjaTask()

	withVikunjaEnv(t, server.URL, func() {
		args, _ := json.Marshal(map[string]any{"id": 1})
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, "deleted") {
			t.Errorf("unexpected result: %s", result)
		}
	})
}

func TestVikunjaTools(t *testing.T) {
	tools := VikunjaTools()
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
		if tool.ShouldInclude == nil {
			t.Errorf("tool %s missing ShouldInclude", tool.Name)
		}
	}

	expected := []string{
		"list_vikunja_tasks", "get_vikunja_task",
		"create_vikunja_task", "update_vikunja_task", "delete_vikunja_task",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}
