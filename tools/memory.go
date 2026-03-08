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

// MemoryTools returns all four memory tools configured with the given config.
func MemoryTools(cfg MemoryConfig) []*yac.Tool {
	return []*yac.Tool{
		CreateMemory(cfg),
		SearchMemories(cfg),
		RecallMemory(cfg),
		RemoveMemory(cfg),
	}
}
