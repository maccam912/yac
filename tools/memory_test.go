package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempMemoryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "memories")
}

// --- CreateMemory tests ---

func TestCreateMemory(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := CreateMemory(MemoryConfig{Dir: dir})

	if tool.Name != "create_memory" {
		t.Errorf("expected name 'create_memory', got %q", tool.Name)
	}

	args, _ := json.Marshal(map[string]any{
		"title":     "Test memory",
		"tags":      []string{"test", "unit"},
		"content":   "This is a test memory with details.",
		"essential": true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "Memory created with ID: ") {
		t.Errorf("unexpected result: %s", result)
	}

	// Extract ID and verify file was written.
	id := strings.TrimPrefix(result, "Memory created with ID: ")
	data, err := os.ReadFile(filepath.Join(dir, id+".md"))
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "title: Test memory") {
		t.Error("file missing title")
	}
	if !strings.Contains(content, "tags: [test, unit]") {
		t.Error("file missing tags")
	}
	if !strings.Contains(content, "essential: true") {
		t.Error("file missing essential flag")
	}
	if !strings.Contains(content, "This is a test memory with details.") {
		t.Error("file missing body content")
	}
}

func TestCreateMemoryMissingTitle(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := CreateMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"title":   "",
		"tags":    []string{},
		"content": "some content",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestCreateMemoryMissingContent(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := CreateMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"title":   "A title",
		"tags":    []string{},
		"content": "",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestCreateMemoryBadJSON(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := CreateMemory(MemoryConfig{Dir: dir})

	_, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// --- SearchMemories tests ---

func seedMemories(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	memories := []struct {
		id, title, body string
		tags            []string
		essential       bool
	}{
		{"aaa111", "How to configure Ollama", "Set up Ollama with multi-GPU support by editing the config file.", []string{"ollama", "gpu", "configuration"}, true},
		{"bbb222", "Go error handling patterns", "Always wrap errors with fmt.Errorf and %w verb for proper error chains.", []string{"go", "patterns", "errors"}, false},
		{"ccc333", "GPU memory optimization", "Use gradient checkpointing and mixed precision to reduce GPU memory.", []string{"gpu", "optimization", "ml"}, false},
		{"ddd444", "Ollama model management", "Use ollama pull, ollama list, and ollama rm to manage models.", []string{"ollama", "models"}, false},
		{"eee555", "Favorite pizza recipe", "Make dough with 00 flour, San Marzano tomatoes, fresh mozzarella.", []string{"cooking", "recipes"}, false},
	}

	for _, m := range memories {
		meta := memoryMeta{
			ID:        m.id,
			Title:     m.title,
			Tags:      m.tags,
			Essential: m.essential,
			Created:   "2026-01-01T00:00:00Z",
		}
		content := renderMemoryFile(meta, m.body)
		if err := os.WriteFile(filepath.Join(dir, m.id+".md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchMemoriesByTag(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"tags": []string{"ollama"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "aaa111") {
		t.Error("expected aaa111 (Ollama config) in results")
	}
	if !strings.Contains(result, "ddd444") {
		t.Error("expected ddd444 (Ollama models) in results")
	}
	if strings.Contains(result, "eee555") {
		t.Error("pizza recipe should not match ollama tag")
	}
}

func TestSearchMemoriesByMultipleTags(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	// Searching for gpu+ollama should rank aaa111 highest (matches both).
	args, _ := json.Marshal(map[string]any{
		"tags": []string{"gpu", "ollama"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// aaa111 matches both tags, should appear first.
	idx1 := strings.Index(result, "aaa111")
	idx2 := strings.Index(result, "ccc333")
	idx3 := strings.Index(result, "ddd444")
	if idx1 < 0 {
		t.Fatal("expected aaa111 in results")
	}
	if idx2 < 0 {
		t.Fatal("expected ccc333 in results")
	}
	if idx3 < 0 {
		t.Fatal("expected ddd444 in results")
	}
	if idx1 > idx2 || idx1 > idx3 {
		t.Error("aaa111 should rank first (matches both tags)")
	}
}

func TestSearchMemoriesByKeyword(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"keyword": "GPU",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ccc333 has "GPU" in title (3pts) + body (1pt) = 4pts
	// aaa111 has "GPU" in body only = 1pt
	if !strings.Contains(result, "ccc333") {
		t.Error("expected ccc333 (GPU memory optimization) in results")
	}
}

func TestSearchMemoriesKeywordTitleWeightedHigher(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"keyword": "GPU memory",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ccc333 "GPU memory optimization" has keyword in title, should rank first.
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.Contains(line, "1.") {
			if !strings.Contains(line, "ccc333") {
				t.Errorf("expected ccc333 to be first result, got: %s", line)
			}
			break
		}
	}
}

func TestSearchMemoriesTagsAndKeyword(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"tags":    []string{"ollama"},
		"keyword": "config",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// aaa111 matches tag (2pts) + keyword in title "configure" (3pts) + keyword in body "config" (1pt) = 6pts
	// ddd444 matches tag (2pts) only = 2pts
	if !strings.Contains(result, "aaa111") {
		t.Error("expected aaa111 in results")
	}
}

func TestSearchMemoriesLimit(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"tags":  []string{"gpu", "ollama", "go", "cooking", "ml"},
		"limit": 2,
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Found 2 matching memories") {
		t.Errorf("expected 2 results, got: %s", result)
	}
}

func TestSearchMemoriesNoMatch(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"keyword": "xyznonexistent",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No matching memories") {
		t.Errorf("expected no matches, got: %s", result)
	}
}

func TestSearchMemoriesRequiresInput(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when no tags or keyword given")
	}
}

func TestSearchMemoriesEmptyDir(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := SearchMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"keyword": "test",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No memories found") {
		t.Errorf("expected no memories message, got: %s", result)
	}
}

// --- RecallMemory tests ---

func TestRecallMemory(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := RecallMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{"id": "aaa111"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "How to configure Ollama") {
		t.Error("expected title in recall result")
	}
	if !strings.Contains(result, "ollama, gpu, configuration") {
		t.Error("expected tags in recall result")
	}
	if !strings.Contains(result, "multi-GPU") {
		t.Error("expected body content in recall result")
	}
	if !strings.Contains(result, "Essential: yes") {
		t.Error("expected essential flag in recall result")
	}
}

func TestRecallMemoryNotFound(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := RecallMemory(MemoryConfig{Dir: dir})

	// Dir doesn't even exist yet.
	args, _ := json.Marshal(map[string]any{"id": "nonexistent"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent memory")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRecallMemoryMissingID(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := RecallMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{"id": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

// --- RemoveMemory tests ---

func TestRemoveMemory(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := RemoveMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{"id": "eee555"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "eee555 removed") {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify file is gone.
	if _, err := os.Stat(filepath.Join(dir, "eee555.md")); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestRemoveMemoryNotFound(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := RemoveMemory(MemoryConfig{Dir: dir})

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]any{"id": "nonexistent"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent memory")
	}
}

func TestRemoveMemoryMissingID(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := RemoveMemory(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{"id": ""})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty id")
	}
}

// --- MemoryTools tests ---

func TestMemoryToolsReturnsAllFour(t *testing.T) {
	dir := tempMemoryDir(t)
	tools := MemoryTools(MemoryConfig{Dir: dir})
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	expected := []string{"create_memory", "search_memories", "recall_memory", "remove_memory"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

// --- EssentialMemories tests ---

func TestEssentialMemories(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)

	titles := EssentialMemories(dir)
	if len(titles) != 1 {
		t.Fatalf("expected 1 essential memory, got %d", len(titles))
	}
	if titles[0] != "How to configure Ollama" {
		t.Errorf("unexpected essential title: %s", titles[0])
	}
}

func TestEssentialMemoriesEmptyDir(t *testing.T) {
	dir := tempMemoryDir(t)
	titles := EssentialMemories(dir)
	if len(titles) != 0 {
		t.Errorf("expected 0 essential memories, got %d", len(titles))
	}
}

// --- Frontmatter parsing tests ---

func TestParseFrontmatter(t *testing.T) {
	input := `---
id: abc123
title: Test Title
tags: [tag1, tag2, tag3]
essential: true
created: 2026-01-01T00:00:00Z
---

This is the body content.`

	meta, body, err := parseFrontmatter(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ID != "abc123" {
		t.Errorf("expected id 'abc123', got %q", meta.ID)
	}
	if meta.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", meta.Title)
	}
	if len(meta.Tags) != 3 || meta.Tags[0] != "tag1" {
		t.Errorf("unexpected tags: %v", meta.Tags)
	}
	if !meta.Essential {
		t.Error("expected essential to be true")
	}
	if body != "This is the body content." {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestParseFrontmatterMissing(t *testing.T) {
	_, _, err := parseFrontmatter("no frontmatter here")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseFrontmatterUnclosed(t *testing.T) {
	_, _, err := parseFrontmatter("---\nid: test\nno closing")
	if err == nil {
		t.Error("expected error for unclosed frontmatter")
	}
}

// --- Integration: create then search then recall then remove ---

func TestMemoryFullLifecycle(t *testing.T) {
	dir := tempMemoryDir(t)
	cfg := MemoryConfig{Dir: dir}

	create := CreateMemory(cfg)
	search := SearchMemories(cfg)
	recall := RecallMemory(cfg)
	remove := RemoveMemory(cfg)

	// Create a memory.
	createArgs, _ := json.Marshal(map[string]any{
		"title":     "Go concurrency patterns",
		"tags":      []string{"go", "concurrency"},
		"content":   "Use goroutines and channels for concurrent work. Select statement for multiplexing.",
		"essential": false,
	})
	result, err := create.Execute(context.Background(), createArgs)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := strings.TrimPrefix(result, "Memory created with ID: ")

	// Search for it.
	searchArgs, _ := json.Marshal(map[string]any{
		"tags": []string{"go"},
	})
	result, err = search.Execute(context.Background(), searchArgs)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(result, id) {
		t.Error("search should find the created memory")
	}
	if !strings.Contains(result, "Go concurrency patterns") {
		t.Error("search should include title")
	}
	// Search should NOT include body content.
	if strings.Contains(result, "goroutines and channels") {
		t.Error("search should not include body content")
	}

	// Recall it.
	recallArgs, _ := json.Marshal(map[string]any{"id": id})
	result, err = recall.Execute(context.Background(), recallArgs)
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if !strings.Contains(result, "Go concurrency patterns") {
		t.Error("recall should include title")
	}
	if !strings.Contains(result, "goroutines and channels") {
		t.Error("recall should include body content")
	}

	// Remove it.
	removeArgs, _ := json.Marshal(map[string]any{"id": id})
	result, err = remove.Execute(context.Background(), removeArgs)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}

	// Verify it's gone.
	_, err = recall.Execute(context.Background(), recallArgs)
	if err == nil {
		t.Error("expected error after removal")
	}
}
