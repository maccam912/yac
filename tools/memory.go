package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// MemoryConfig configures memory tools.
type MemoryConfig struct {
	// Dir is the directory where memory files are stored.
	// Each memory is a markdown file with YAML frontmatter.
	Dir string
}

// frontmatter fields stored at the top of each memory file.
type memoryMeta struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	Essential bool     `json:"essential"`
	Created   string   `json:"created"`
}

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// parseFrontmatter parses a memory file's content into metadata and body.
func parseFrontmatter(data string) (memoryMeta, string, error) {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "---") {
		return memoryMeta{}, "", fmt.Errorf("missing frontmatter")
	}

	rest := data[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return memoryMeta{}, "", fmt.Errorf("unclosed frontmatter")
	}

	fmBlock := strings.TrimSpace(rest[:end])
	body := strings.TrimSpace(rest[end+4:]) // skip past "\n---"

	meta := memoryMeta{}
	// Parse simple YAML-like frontmatter line by line.
	for _, line := range strings.Split(fmBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "id":
			meta.ID = val
		case "title":
			meta.Title = val
		case "essential":
			meta.Essential = val == "true"
		case "created":
			meta.Created = val
		case "tags":
			meta.Tags = parseTags(val)
		}
	}

	return meta, body, nil
}

// parseTags parses a YAML-style inline list like "[tag1, tag2]".
func parseTags(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}

// renderMemoryFile produces the markdown file content for a memory.
func renderMemoryFile(meta memoryMeta, body string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("id: " + meta.ID + "\n")
	sb.WriteString("title: " + meta.Title + "\n")
	sb.WriteString("tags: [" + strings.Join(meta.Tags, ", ") + "]\n")
	if meta.Essential {
		sb.WriteString("essential: true\n")
	} else {
		sb.WriteString("essential: false\n")
	}
	sb.WriteString("created: " + meta.Created + "\n")
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	sb.WriteString("\n")
	return sb.String()
}

// loadAllMemories reads and parses all .md files in the memory dir.
func loadAllMemories(dir string) ([]memoryMeta, map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var metas []memoryMeta
	bodies := make(map[string]string)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		meta, body, err := parseFrontmatter(string(data))
		if err != nil {
			continue
		}
		metas = append(metas, meta)
		bodies[meta.ID] = body
	}

	return metas, bodies, nil
}

// EssentialMemories returns the titles of all memories marked as essential.
// This is useful for injecting into system prompts.
func EssentialMemories(dir string) []string {
	metas, _, err := loadAllMemories(dir)
	if err != nil {
		return nil
	}
	var titles []string
	for _, m := range metas {
		if m.Essential {
			titles = append(titles, m.Title)
		}
	}
	return titles
}

// CreateMemory returns a tool that creates a new memory file.
func CreateMemory(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "create_memory",
		Description: "Create a new memory. Stores a titled piece of information with tags for later retrieval. Use this to remember important facts, decisions, user preferences, or any information worth recalling later.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "A short summary or title explaining what this memory is about.",
				},
				"tags": map[string]any{
					"type":        "array",
					"description": "Tags for categorizing this memory. Reuse existing tags when they fit; create new ones only for genuine gaps.",
					"items": map[string]any{
						"type": "string",
					},
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The detailed content of the memory. This can be as long as needed.",
				},
				"essential": map[string]any{
					"type":        "boolean",
					"description": "If true, this memory's title will always be included in context so it is visible on every prompt. Use sparingly for truly critical information.",
				},
			},
			"required": []string{"title", "tags", "content"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Title     string   `json:"title"`
				Tags      []string `json:"tags"`
				Content   string   `json:"content"`
				Essential bool     `json:"essential"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Title == "" {
				return "", fmt.Errorf("title is required")
			}
			if params.Content == "" {
				return "", fmt.Errorf("content is required")
			}

			// Ensure directory exists.
			if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
				return "", fmt.Errorf("failed to create memory directory: %w", err)
			}

			id := randomID()
			meta := memoryMeta{
				ID:        id,
				Title:     params.Title,
				Tags:      params.Tags,
				Essential: params.Essential,
				Created:   time.Now().UTC().Format(time.RFC3339),
			}

			fileContent := renderMemoryFile(meta, params.Content)
			filePath := filepath.Join(cfg.Dir, id+".md")
			if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
				return "", fmt.Errorf("failed to write memory file: %w", err)
			}

			return fmt.Sprintf("Memory created with ID: %s", id), nil
		},
	}
}

// SearchMemories returns a tool that searches memories by tags and/or keywords.
func SearchMemories(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "search_memories",
		Description: "Search stored memories by tags and/or keywords. Returns titles, tags, and IDs of matching memories (most relevant first). Use recall_memory with an ID to get full details.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"tags": map[string]any{
					"type":        "array",
					"description": "Tags to filter by. Memories matching more of these tags rank higher.",
					"items": map[string]any{
						"type": "string",
					},
				},
				"keyword": map[string]any{
					"type":        "string",
					"description": "Keyword to search for in titles and content. Title matches are weighted higher.",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Maximum number of results to return. Defaults to 3.",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Tags    []string `json:"tags"`
				Keyword string   `json:"keyword"`
				Limit   int      `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if len(params.Tags) == 0 && params.Keyword == "" {
				return "", fmt.Errorf("at least one of tags or keyword is required")
			}

			if params.Limit <= 0 {
				params.Limit = 3
			}

			metas, bodies, err := loadAllMemories(cfg.Dir)
			if err != nil {
				return "", fmt.Errorf("failed to load memories: %w", err)
			}

			if len(metas) == 0 {
				return "No memories found.", nil
			}

			type scored struct {
				meta  memoryMeta
				score float64
			}

			var results []scored

			for _, m := range metas {
				var score float64

				// Tag scoring: each matching tag adds 2 points.
				if len(params.Tags) > 0 {
					tagSet := make(map[string]bool)
					for _, t := range m.Tags {
						tagSet[strings.ToLower(t)] = true
					}
					for _, t := range params.Tags {
						if tagSet[strings.ToLower(t)] {
							score += 2.0
						}
					}
				}

				// Keyword scoring: title match = 3 points, body match = 1 point.
				if params.Keyword != "" {
					kw := strings.ToLower(params.Keyword)
					if strings.Contains(strings.ToLower(m.Title), kw) {
						score += 3.0
					}
					if body, ok := bodies[m.ID]; ok {
						if strings.Contains(strings.ToLower(body), kw) {
							score += 1.0
						}
					}
				}

				if score > 0 {
					results = append(results, scored{meta: m, score: score})
				}
			}

			if len(results) == 0 {
				return "No matching memories found.", nil
			}

			// Sort by score descending, then by created time descending (newer first).
			sort.Slice(results, func(i, j int) bool {
				if results[i].score != results[j].score {
					return results[i].score > results[j].score
				}
				return results[i].meta.Created > results[j].meta.Created
			})

			if params.Limit < len(results) {
				results = results[:params.Limit]
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Found %d matching memories:\n\n", len(results))
			for i, r := range results {
				fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, r.meta.ID, r.meta.Title)
				if len(r.meta.Tags) > 0 {
					fmt.Fprintf(&sb, "   Tags: %s\n", strings.Join(r.meta.Tags, ", "))
				}
			}

			return sb.String(), nil
		},
	}
}

// RecallMemory returns a tool that retrieves the full content of a memory by ID.
func RecallMemory(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "recall_memory",
		Description: "Retrieve the full details of a memory by its ID. Returns the title, tags, and complete content.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The ID of the memory to recall.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ID == "" {
				return "", fmt.Errorf("id is required")
			}

			filePath := filepath.Join(cfg.Dir, params.ID+".md")
			data, err := os.ReadFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("memory not found: %s", params.ID)
				}
				return "", fmt.Errorf("failed to read memory: %w", err)
			}

			meta, body, err := parseFrontmatter(string(data))
			if err != nil {
				return "", fmt.Errorf("failed to parse memory: %w", err)
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Title: %s\n", meta.Title)
			if len(meta.Tags) > 0 {
				fmt.Fprintf(&sb, "Tags: %s\n", strings.Join(meta.Tags, ", "))
			}
			if meta.Essential {
				sb.WriteString("Essential: yes\n")
			}
			fmt.Fprintf(&sb, "Created: %s\n", meta.Created)
			fmt.Fprintf(&sb, "\n%s", body)

			return sb.String(), nil
		},
	}
}

// memoryPageSize is the number of memories per page in ListMemories.
const memoryPageSize = 50

// countMemories returns the number of .md files in the memory directory.
func countMemories(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

// ListMemories returns a tool that lists all memories with pagination.
// Memories are sorted in reverse chronological order (newest first).
// The tool description is dynamic and includes the current total memory count.
func ListMemories(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name: "list_memories",
		DynamicDescription: func() string {
			n := countMemories(cfg.Dir)
			pages := (n + memoryPageSize - 1) / memoryPageSize
			if pages < 1 {
				pages = 1
			}
			return fmt.Sprintf(
				"List all memories sorted by newest first (%d memories total, %d pages of up to %d). "+
					"Returns ID, title, tags, and created date for each memory. "+
					"Use the page parameter to paginate (default: page 1).",
				n, pages, memoryPageSize,
			)
		},
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"page": map[string]any{
					"type":        "number",
					"description": "Page number (1-based). Defaults to 1.",
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Page int `json:"page"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.Page < 1 {
				params.Page = 1
			}

			metas, _, err := loadAllMemories(cfg.Dir)
			if err != nil {
				return "", fmt.Errorf("failed to load memories: %w", err)
			}

			if len(metas) == 0 {
				return "No memories stored yet.", nil
			}

			// Sort by created time descending (newest first).
			sort.Slice(metas, func(i, j int) bool {
				return metas[i].Created > metas[j].Created
			})

			total := len(metas)
			totalPages := (total + memoryPageSize - 1) / memoryPageSize
			if params.Page > totalPages {
				return fmt.Sprintf("Page %d is out of range (total pages: %d).", params.Page, totalPages), nil
			}

			start := (params.Page - 1) * memoryPageSize
			end := start + memoryPageSize
			if end > total {
				end = total
			}
			page := metas[start:end]

			var sb strings.Builder
			fmt.Fprintf(&sb, "Memories (page %d of %d, %d total):\n\n", params.Page, totalPages, total)
			for i, m := range page {
				num := start + i + 1
				fmt.Fprintf(&sb, "%d. [%s] %s\n", num, m.ID, m.Title)
				if len(m.Tags) > 0 {
					fmt.Fprintf(&sb, "   Tags: %s\n", strings.Join(m.Tags, ", "))
				}
				fmt.Fprintf(&sb, "   Created: %s\n", m.Created)
			}

			if params.Page < totalPages {
				fmt.Fprintf(&sb, "\nUse page=%d to see more.", params.Page+1)
			}

			return sb.String(), nil
		},
	}
}

// EditMemory returns a tool that updates a memory in place by ID.
// Only the fields provided are changed; omitted fields keep their current values.
func EditMemory(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "edit_memory",
		Description: "Update an existing memory in place by its ID. Only provide the fields you want to change — omitted fields are preserved. This is more efficient than removing and recreating a memory.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The ID of the memory to edit.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "New title. Omit to keep the current title.",
				},
				"tags": map[string]any{
					"type":        "array",
					"description": "New tags (replaces all existing tags). Omit to keep current tags.",
					"items": map[string]any{
						"type": "string",
					},
				},
				"content": map[string]any{
					"type":        "string",
					"description": "New content. Omit to keep the current content.",
				},
				"essential": map[string]any{
					"type":        "boolean",
					"description": "New essential flag. Omit to keep the current value.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			// Use a raw map so we can distinguish between "field not provided"
			// and "field provided as zero value".
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(args, &raw); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			idRaw, ok := raw["id"]
			if !ok {
				return "", fmt.Errorf("id is required")
			}
			var id string
			if err := json.Unmarshal(idRaw, &id); err != nil || id == "" {
				return "", fmt.Errorf("id is required")
			}

			// Read existing memory.
			filePath := filepath.Join(cfg.Dir, id+".md")
			data, err := os.ReadFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("memory not found: %s", id)
				}
				return "", fmt.Errorf("failed to read memory: %w", err)
			}

			meta, body, err := parseFrontmatter(string(data))
			if err != nil {
				return "", fmt.Errorf("failed to parse memory: %w", err)
			}

			// Apply updates for provided fields.
			var changed []string
			if v, ok := raw["title"]; ok {
				var title string
				if err := json.Unmarshal(v, &title); err == nil && title != "" {
					meta.Title = title
					changed = append(changed, "title")
				}
			}
			if v, ok := raw["tags"]; ok {
				var tags []string
				if err := json.Unmarshal(v, &tags); err == nil {
					meta.Tags = tags
					changed = append(changed, "tags")
				}
			}
			if v, ok := raw["content"]; ok {
				var content string
				if err := json.Unmarshal(v, &content); err == nil && content != "" {
					body = content
					changed = append(changed, "content")
				}
			}
			if v, ok := raw["essential"]; ok {
				var essential bool
				if err := json.Unmarshal(v, &essential); err == nil {
					meta.Essential = essential
					changed = append(changed, "essential")
				}
			}

			if len(changed) == 0 {
				return "No changes provided.", nil
			}

			// Write back.
			fileContent := renderMemoryFile(meta, body)
			if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
				return "", fmt.Errorf("failed to write memory: %w", err)
			}

			return fmt.Sprintf("Memory %s updated (%s).", id, strings.Join(changed, ", ")), nil
		},
	}
}

// RemoveMemory returns a tool that deletes a memory by ID.
func RemoveMemory(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "remove_memory",
		Description: "Delete a memory by its ID. This permanently removes the memory file.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The ID of the memory to remove.",
				},
			},
			"required": []string{"id"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			if params.ID == "" {
				return "", fmt.Errorf("id is required")
			}

			filePath := filepath.Join(cfg.Dir, params.ID+".md")
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return "", fmt.Errorf("memory not found: %s", params.ID)
			}

			if err := os.Remove(filePath); err != nil {
				return "", fmt.Errorf("failed to remove memory: %w", err)
			}

			return fmt.Sprintf("Memory %s removed.", params.ID), nil
		},
	}
}

// MemoryTools returns all memory tools configured with the given config.
func MemoryTools(cfg MemoryConfig) []*yac.Tool {
	return []*yac.Tool{
		CreateMemory(cfg),
		ListMemories(cfg),
		SearchMemories(cfg),
		RecallMemory(cfg),
		EditMemory(cfg),
		RemoveMemory(cfg),
		ConsolidateMemories(cfg),
	}
}
