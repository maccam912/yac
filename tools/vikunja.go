package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// vikunjaRequest makes an authenticated HTTP request to the Vikunja API.
func vikunjaRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	baseURL := strings.TrimSuffix(os.Getenv("VIKUNJA_URL"), "/")
	apiKey := os.Getenv("VIKUNJA_API_KEY")

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+"/api/v1"+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func vikunjaShouldInclude() bool {
	return os.Getenv("VIKUNJA_API_KEY") != "" && os.Getenv("VIKUNJA_URL") != ""
}

// ListVikunjaTasks returns a tool that lists Vikunja tasks showing only
// ID, title, project, done status, priority, and due date — keeping output compact.
func ListVikunjaTasks() *yac.Tool {
	return &yac.Tool{
		Name:        "list_vikunja_tasks",
		Description: "List tasks from Vikunja. By default only shows incomplete tasks. Set include_completed=true to also see completed tasks. Returns a compact summary (ID, title, project, done, priority, due date) for each task. Use get_vikunja_task with an ID to see full details.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"page": map[string]any{
					"type":        "number",
					"description": "Page number (1-based). Defaults to 1. Each page returns up to 50 tasks.",
				},
				"filter": map[string]any{
					"type":        "string",
					"description": "Optional additional filter string using Vikunja filter syntax.",
				},
				"include_completed": map[string]any{
					"type":        "boolean",
					"description": "If true, include completed tasks in the results. Defaults to false (only incomplete tasks).",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Page             int    `json:"page"`
				Filter           string `json:"filter"`
				IncludeCompleted bool   `json:"include_completed"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Page < 1 {
				params.Page = 1
			}

			path := fmt.Sprintf("/tasks/all?page=%d&per_page=50&sort_by[]=id&order_by[]=desc", params.Page)
			var filter string
			if !params.IncludeCompleted {
				filter = "done = false"
				if params.Filter != "" {
					filter += " && " + params.Filter
				}
			} else {
				filter = params.Filter
			}
			if filter != "" {
				path += "&filter=" + url.QueryEscape(filter)
			}

			data, err := vikunjaRequest(ctx, "GET", path, nil)
			if err != nil {
				return "", err
			}

			var tasks []struct {
				ID      int    `json:"id"`
				Title   string `json:"title"`
				Project struct {
					Title string `json:"title"`
				} `json:"project"`
				ProjectID int    `json:"project_id"`
				Done      bool   `json:"done"`
				Priority  int    `json:"priority"`
				DueDate   string `json:"due_date"`
			}
			if err := json.Unmarshal(data, &tasks); err != nil {
				return "", fmt.Errorf("failed to parse tasks: %w", err)
			}

			if len(tasks) == 0 {
				return "No tasks found.", nil
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Tasks (page %d):\n\n", params.Page)
			for _, t := range tasks {
				status := "[ ]"
				if t.Done {
					status = "[x]"
				}
				line := fmt.Sprintf("%s #%d: %s", status, t.ID, t.Title)
				switch {
				case t.Project.Title != "":
					line += fmt.Sprintf(" (project: %s)", t.Project.Title)
				case t.ProjectID > 0:
					line += fmt.Sprintf(" (project: #%d)", t.ProjectID)
				}
				if t.Priority > 0 {
					line += fmt.Sprintf(" (priority: %d)", t.Priority)
				}
				if t.DueDate != "" && !strings.HasPrefix(t.DueDate, "0001") {
					line += fmt.Sprintf(" (due: %s)", t.DueDate[:10])
				}
				sb.WriteString(line + "\n")
			}

			if len(tasks) == 50 {
				fmt.Fprintf(&sb, "\nMore tasks may exist. Use page=%d to see next page.", params.Page+1)
			}

			return sb.String(), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// GetVikunjaTask returns a tool that retrieves full details of a single task.
func GetVikunjaTask() *yac.Tool {
	return &yac.Tool{
		Name:        "get_vikunja_task",
		Description: "Get full details of a Vikunja task by its ID. Returns title, description, status, priority, dates, labels, and assignees.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "number",
					"description": "The numeric ID of the task.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ID == 0 {
				return "", fmt.Errorf("id is required")
			}

			data, err := vikunjaRequest(ctx, "GET", fmt.Sprintf("/tasks/%d", params.ID), nil)
			if err != nil {
				return "", err
			}

			var task struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Done        bool   `json:"done"`
				DoneAt      string `json:"done_at"`
				DueDate     string `json:"due_date"`
				StartDate   string `json:"start_date"`
				EndDate     string `json:"end_date"`
				Priority    int    `json:"priority"`
				PercentDone int    `json:"percent_done"`
				ProjectID   int    `json:"project_id"`
				HexColor    string `json:"hex_color"`
				Identifier  string `json:"identifier"`
				IsFavorite  bool   `json:"is_favorite"`
				Created     string `json:"created"`
				Updated     string `json:"updated"`
				Labels      []struct {
					ID    int    `json:"id"`
					Title string `json:"title"`
				} `json:"labels"`
				Assignees []struct {
					ID       int    `json:"id"`
					Username string `json:"username"`
				} `json:"assignees"`
			}
			if err := json.Unmarshal(data, &task); err != nil {
				return "", fmt.Errorf("failed to parse task: %w", err)
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Task #%d: %s\n", task.ID, task.Title)
			if task.Identifier != "" {
				fmt.Fprintf(&sb, "Identifier: %s\n", task.Identifier)
			}
			fmt.Fprintf(&sb, "Done: %v\n", task.Done)
			if task.Priority > 0 {
				fmt.Fprintf(&sb, "Priority: %d\n", task.Priority)
			}
			if task.PercentDone > 0 {
				fmt.Fprintf(&sb, "Progress: %d%%\n", task.PercentDone)
			}
			fmt.Fprintf(&sb, "Project ID: %d\n", task.ProjectID)

			formatDate := func(label, val string) {
				if val != "" && !strings.HasPrefix(val, "0001") {
					fmt.Fprintf(&sb, "%s: %s\n", label, val)
				}
			}
			formatDate("Due", task.DueDate)
			formatDate("Start", task.StartDate)
			formatDate("End", task.EndDate)
			formatDate("Done at", task.DoneAt)
			formatDate("Created", task.Created)
			formatDate("Updated", task.Updated)

			if len(task.Labels) > 0 {
				labels := make([]string, len(task.Labels))
				for i, l := range task.Labels {
					labels[i] = l.Title
				}
				fmt.Fprintf(&sb, "Labels: %s\n", strings.Join(labels, ", "))
			}
			if len(task.Assignees) > 0 {
				assignees := make([]string, len(task.Assignees))
				for i, a := range task.Assignees {
					assignees[i] = a.Username
				}
				fmt.Fprintf(&sb, "Assignees: %s\n", strings.Join(assignees, ", "))
			}
			if task.IsFavorite {
				sb.WriteString("Favorite: yes\n")
			}
			if task.Description != "" {
				fmt.Fprintf(&sb, "\nDescription:\n%s", task.Description)
			}

			return sb.String(), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// CreateVikunjaTask returns a tool that creates a new task in Vikunja.
func CreateVikunjaTask() *yac.Tool {
	return &yac.Tool{
		Name:        "create_vikunja_task",
		Description: "Create a new task in Vikunja. Requires a title and project ID. Returns the created task's ID.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "The title of the task.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of the task.",
				},
				"project_id": map[string]any{
					"type":        "number",
					"description": "The ID of the project to create the task in.",
				},
				"priority": map[string]any{
					"type":        "number",
					"description": "Priority level (0-5, where 5 is highest).",
				},
				"due_date": map[string]any{
					"type":        "string",
					"description": "Due date in ISO 8601 format, e.g. '2026-03-15T00:00:00Z'.",
				},
			},
			"required": []string{"title", "project_id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				ProjectID   int    `json:"project_id"`
				Priority    int    `json:"priority"`
				DueDate     string `json:"due_date"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Title == "" {
				return "", fmt.Errorf("title is required")
			}
			if params.ProjectID == 0 {
				return "", fmt.Errorf("project_id is required")
			}

			body := map[string]any{
				"title": params.Title,
			}
			if params.Description != "" {
				body["description"] = params.Description
			}
			if params.Priority > 0 {
				body["priority"] = params.Priority
			}
			if params.DueDate != "" {
				body["due_date"] = params.DueDate
			}

			data, err := vikunjaRequest(ctx, "PUT", fmt.Sprintf("/projects/%d/tasks", params.ProjectID), body)
			if err != nil {
				return "", err
			}

			var created struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			}
			if err := json.Unmarshal(data, &created); err != nil {
				return "", fmt.Errorf("failed to parse response: %w", err)
			}

			return fmt.Sprintf("Task created with ID: %d (%s)", created.ID, created.Title), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// UpdateVikunjaTask returns a tool that updates an existing task.
// Only provided fields are changed.
func UpdateVikunjaTask() *yac.Tool {
	return &yac.Tool{
		Name:        "update_vikunja_task",
		Description: "Update an existing Vikunja task by ID. Only provide the fields you want to change. Use this to mark tasks done, change priority, update descriptions, etc.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "number",
					"description": "The numeric ID of the task to update.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "New title. Omit to keep current.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "New description. Omit to keep current.",
				},
				"done": map[string]any{
					"type":        "boolean",
					"description": "Set to true to mark complete, false to reopen.",
				},
				"priority": map[string]any{
					"type":        "number",
					"description": "New priority (0-5).",
				},
				"due_date": map[string]any{
					"type":        "string",
					"description": "New due date in ISO 8601 format.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			// Use raw map to distinguish provided vs omitted fields.
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			idRaw, ok := raw["id"]
			if !ok {
				return "", fmt.Errorf("id is required")
			}
			var id int
			if err := json.Unmarshal(idRaw, &id); err != nil || id == 0 {
				return "", fmt.Errorf("id is required")
			}

			body := make(map[string]any)
			var changed []string

			if v, ok := raw["title"]; ok {
				var s string
				if err := json.Unmarshal(v, &s); err == nil && s != "" {
					body["title"] = s
					changed = append(changed, "title")
				}
			}
			if v, ok := raw["description"]; ok {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					body["description"] = s
					changed = append(changed, "description")
				}
			}
			if v, ok := raw["done"]; ok {
				var b bool
				if err := json.Unmarshal(v, &b); err == nil {
					body["done"] = b
					changed = append(changed, "done")
				}
			}
			if v, ok := raw["priority"]; ok {
				var n int
				if err := json.Unmarshal(v, &n); err == nil {
					body["priority"] = n
					changed = append(changed, "priority")
				}
			}
			if v, ok := raw["due_date"]; ok {
				var s string
				if err := json.Unmarshal(v, &s); err == nil && s != "" {
					body["due_date"] = s
					changed = append(changed, "due_date")
				}
			}

			if len(changed) == 0 {
				return "No changes provided.", nil
			}

			_, err := vikunjaRequest(ctx, "POST", fmt.Sprintf("/tasks/%d", id), body)
			if err != nil {
				return "", err
			}

			return fmt.Sprintf("Task #%d updated (%s).", id, strings.Join(changed, ", ")), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// DeleteVikunjaTask returns a tool that deletes a task by ID.
func DeleteVikunjaTask() *yac.Tool {
	return &yac.Tool{
		Name:        "delete_vikunja_task",
		Description: "Delete a Vikunja task by its ID. This permanently removes the task.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "number",
					"description": "The numeric ID of the task to delete.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ID == 0 {
				return "", fmt.Errorf("id is required")
			}

			_, err := vikunjaRequest(ctx, "DELETE", fmt.Sprintf("/tasks/%d", params.ID), nil)
			if err != nil {
				return "", err
			}

			return fmt.Sprintf("Task #%d deleted.", params.ID), nil
		},
		ShouldInclude: vikunjaShouldInclude,
	}
}

// VikunjaTools returns all Vikunja tools. All tools are conditionally included
// based on the VIKUNJA_API_KEY and VIKUNJA_URL environment variables.
func VikunjaTools() []*yac.Tool {
	return []*yac.Tool{
		ListVikunjaTasks(),
		GetVikunjaTask(),
		CreateVikunjaTask(),
		UpdateVikunjaTask(),
		DeleteVikunjaTask(),
	}
}
