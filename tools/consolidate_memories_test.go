package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidateFindAllMemories(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	if tool.Name != "consolidate_memories" {
		t.Errorf("expected name 'consolidate_memories', got %q", tool.Name)
	}

	// Find with no filters — scans all memories.
	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// aaa111 and ddd444 share "ollama" tag (2pts) + both have "Ollama" in title (3pts) = 5pts >= 4.
	if !strings.Contains(result, "aaa111") || !strings.Contains(result, "ddd444") {
		t.Errorf("expected ollama memories grouped together, got: %s", result)
	}
}

func TestConsolidateFindWithTagFilter(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action": "find",
		"tags":   []string{"ollama"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "aaa111") {
		t.Error("expected aaa111 in results")
	}
	if !strings.Contains(result, "ddd444") {
		t.Error("expected ddd444 in results")
	}
	// Pizza recipe shouldn't appear.
	if strings.Contains(result, "eee555") {
		t.Error("pizza recipe should not appear in ollama filter")
	}
}

func TestConsolidateFindWithKeywordFilter(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action":  "find",
		"keyword": "Ollama",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "aaa111") || !strings.Contains(result, "ddd444") {
		t.Errorf("expected ollama memories in results, got: %s", result)
	}
}

func TestConsolidateFindNoSimilar(t *testing.T) {
	dir := tempMemoryDir(t)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two completely unrelated memories.
	memories := []struct {
		id, title, body string
		tags            []string
	}{
		{"xxx111", "Apples", "Information about apples.", []string{"fruit"}},
		{"xxx222", "Databases", "Information about databases.", []string{"tech"}},
	}
	for _, m := range memories {
		meta := memoryMeta{ID: m.id, Title: m.title, Tags: m.tags, Created: "2026-01-01T00:00:00Z"}
		content := renderMemoryFile(meta, m.body)
		os.WriteFile(filepath.Join(dir, m.id+".md"), []byte(content), 0644)
	}

	tool := ConsolidateMemories(MemoryConfig{Dir: dir})
	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No similar") {
		t.Errorf("expected no similar groups, got: %s", result)
	}
}

func TestConsolidateFindEmpty(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No memories found") {
		t.Errorf("expected no memories message, got: %s", result)
	}
}

func TestConsolidateFindTooFewAfterFilter(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	// Only one memory matches "cooking" tag.
	args, _ := json.Marshal(map[string]any{
		"tags": []string{"cooking"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Not enough") {
		t.Errorf("expected 'not enough' message, got: %s", result)
	}
}

func TestConsolidateMerge(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action": "merge",
		"merge_groups": []map[string]any{
			{
				"source_ids": []string{"aaa111", "ddd444"},
				"title":      "Ollama Guide",
				"tags":       []string{"ollama", "gpu", "configuration", "models"},
				"content":    "Combined guide for Ollama setup, configuration, and model management.",
				"essential":  true,
			},
		},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Merged 2 memories") {
		t.Errorf("expected merge summary, got: %s", result)
	}
	if !strings.Contains(result, "aaa111") || !strings.Contains(result, "ddd444") {
		t.Errorf("expected source IDs in result, got: %s", result)
	}

	// Source files should be deleted.
	if _, err := os.Stat(filepath.Join(dir, "aaa111.md")); !os.IsNotExist(err) {
		t.Error("expected aaa111.md to be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "ddd444.md")); !os.IsNotExist(err) {
		t.Error("expected ddd444.md to be deleted")
	}

	// Other memories should still exist.
	if _, err := os.Stat(filepath.Join(dir, "bbb222.md")); err != nil {
		t.Error("bbb222.md should still exist")
	}

	// New merged memory should exist.
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if e.Name() != "bbb222.md" && e.Name() != "ccc333.md" && e.Name() != "eee555.md" {
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			content := string(data)
			if strings.Contains(content, "title: Ollama Guide") &&
				strings.Contains(content, "essential: true") &&
				strings.Contains(content, "Combined guide") {
				found = true
			}
		}
	}
	if !found {
		t.Error("merged memory file not found")
	}
}

func TestConsolidateMergeNonexistentID(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action": "merge",
		"merge_groups": []map[string]any{
			{
				"source_ids": []string{"aaa111", "nonexistent"},
				"title":      "Test",
				"tags":       []string{"test"},
				"content":    "Test content.",
			},
		},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for nonexistent source ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}

	// aaa111 should NOT have been deleted (validation happens before any changes).
	if _, err := os.Stat(filepath.Join(dir, "aaa111.md")); err != nil {
		t.Error("aaa111.md should still exist after failed merge")
	}
}

func TestConsolidateMergeTooFewSources(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action": "merge",
		"merge_groups": []map[string]any{
			{
				"source_ids": []string{"aaa111"},
				"title":      "Test",
				"tags":       []string{},
				"content":    "Test.",
			},
		},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for single source ID")
	}
}

func TestConsolidateMergeEmptyGroups(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{
		"action":       "merge",
		"merge_groups": []map[string]any{},
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for empty merge_groups")
	}
}

func TestConsolidateUnknownAction(t *testing.T) {
	dir := tempMemoryDir(t)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	args, _ := json.Marshal(map[string]any{"action": "delete"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestSimilarityScore(t *testing.T) {
	a := memoryMeta{Title: "Ollama config", Tags: []string{"ollama", "gpu"}}
	b := memoryMeta{Title: "Ollama models", Tags: []string{"ollama", "models"}}

	score := similarityScore(a, b, "config body", "models body")
	// 1 shared tag (ollama) = 2pts + shared title word "ollama" (4+ chars) = 3pts + shared body word "body" (4+ chars) = 1pt = 6pts.
	if score != 6 {
		t.Errorf("expected score 6, got %d", score)
	}
}

func TestSimilarityScoreTitleOverlap(t *testing.T) {
	a := memoryMeta{Title: "GPU optimization", Tags: []string{"gpu"}}
	b := memoryMeta{Title: "GPU optimization techniques", Tags: []string{"gpu", "ml"}}

	score := similarityScore(a, b, "", "")
	// 1 shared tag = 2pts, "gpu optimization" is contained in "gpu optimization techniques" = 3pts, total 5.
	if score != 5 {
		t.Errorf("expected score 5, got %d", score)
	}
}

func TestGroupSimilarTransitive(t *testing.T) {
	// A~B and B~C should group A+B+C together.
	metas := []memoryMeta{
		{ID: "a", Title: "Go patterns", Tags: []string{"go", "patterns"}},
		{ID: "b", Title: "Go error patterns", Tags: []string{"go", "patterns", "errors"}},
		{ID: "c", Title: "Go error handling", Tags: []string{"go", "errors"}},
		{ID: "d", Title: "Pizza recipe", Tags: []string{"cooking"}},
	}
	bodies := map[string]string{
		"a": "patterns body",
		"b": "error patterns body",
		"c": "error handling body",
		"d": "pizza body",
	}

	groups := groupSimilar(metas, bodies, 4)
	// A and B share "go"+"patterns" = 4pts, "go patterns" is substring of "go error patterns" = +3 = 7pts.
	// B and C share "go"+"errors" = 4pts.
	// So A, B, C should be in one group. D should not be grouped.
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("expected 3 members in group, got %d", len(groups[0]))
	}
}

func TestConsolidateFullLifecycle(t *testing.T) {
	dir := tempMemoryDir(t)
	seedMemories(t, dir)
	tool := ConsolidateMemories(MemoryConfig{Dir: dir})

	// Step 1: Find similar memories.
	findArgs, _ := json.Marshal(map[string]any{"action": "find"})
	result, err := tool.Execute(context.Background(), findArgs)
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if !strings.Contains(result, "Group") {
		t.Fatalf("expected groups in find result, got: %s", result)
	}

	// Step 2: Merge the ollama group.
	mergeArgs, _ := json.Marshal(map[string]any{
		"action": "merge",
		"merge_groups": []map[string]any{
			{
				"source_ids": []string{"aaa111", "ddd444"},
				"title":      "Complete Ollama Guide",
				"tags":       []string{"ollama", "gpu", "configuration", "models"},
				"content":    "Setup, config, and model management for Ollama.",
			},
		},
	})
	result, err = tool.Execute(context.Background(), mergeArgs)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !strings.Contains(result, "Merged 2") {
		t.Errorf("expected merge summary, got: %s", result)
	}

	// Verify: 4 files remain (5 original - 2 deleted + 1 new).
	entries, _ := os.ReadDir(dir)
	mdCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdCount++
		}
	}
	if mdCount != 4 {
		t.Errorf("expected 4 memory files after merge, got %d", mdCount)
	}
}
