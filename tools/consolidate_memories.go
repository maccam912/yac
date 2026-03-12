package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maccam912/yac"
)

// similarityScore computes a simple pairwise similarity between two memories.
// Shared tags = +2 each, title substring overlap = +3, body substring overlap = +1.
func similarityScore(a, b memoryMeta, bodyA, bodyB string) int {
	score := 0

	// Shared tags.
	bTags := make(map[string]bool, len(b.Tags))
	for _, t := range b.Tags {
		bTags[strings.ToLower(t)] = true
	}
	for _, t := range a.Tags {
		if bTags[strings.ToLower(t)] {
			score += 2
		}
	}

	// Title word overlap: if titles share any significant word (4+ chars), +3 points.
	aTitle := strings.ToLower(a.Title)
	bTitle := strings.ToLower(b.Title)
	if aTitle != "" && bTitle != "" {
		aWords := strings.Fields(aTitle)
		bWordSet := make(map[string]bool)
		for _, w := range strings.Fields(bTitle) {
			if len(w) >= 4 {
				bWordSet[w] = true
			}
		}
		for _, w := range aWords {
			if len(w) >= 4 && bWordSet[w] {
				score += 3
				break
			}
		}
	}

	// Body word overlap: if bodies share any significant word (4+ chars), +1 point.
	aBody := strings.ToLower(bodyA)
	bBody := strings.ToLower(bodyB)
	if aBody != "" && bBody != "" {
		aWords := strings.Fields(aBody)
		bWordSet := make(map[string]bool)
		for _, w := range strings.Fields(bBody) {
			if len(w) >= 4 {
				bWordSet[w] = true
			}
		}
		for _, w := range aWords {
			if len(w) >= 4 && bWordSet[w] {
				score += 1
				break
			}
		}
	}

	return score
}

// groupSimilar groups memories using union-find based on pairwise similarity.
// Returns groups of 2+ memories that have similarity >= threshold.
func groupSimilar(metas []memoryMeta, bodies map[string]string, threshold int) [][]int {
	n := len(metas)
	if n < 2 {
		return nil
	}

	// Union-find.
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		px, py := find(x), find(y)
		if px != py {
			parent[px] = py
		}
	}

	// Pairwise comparison.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			s := similarityScore(metas[i], metas[j], bodies[metas[i].ID], bodies[metas[j].ID])
			if s >= threshold {
				union(i, j)
			}
		}
	}

	// Collect groups.
	groups := make(map[int][]int)
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	// Only return groups with 2+ members.
	var result [][]int
	for _, members := range groups {
		if len(members) >= 2 {
			result = append(result, members)
		}
	}
	return result
}

const similarityThreshold = 4

// ConsolidateMemories returns a tool that finds duplicate/similar memories and merges them.
func ConsolidateMemories(cfg MemoryConfig) *yac.Tool {
	return &yac.Tool{
		Name:        "consolidate_memories",
		Description: "Find and merge duplicate or similar memories. Use action \"find\" to discover groups of similar memories, then action \"merge\" to combine them. This reduces clutter by turning many similar memories into consolidated ones.",
		Parameters: yac.Schema{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: \"find\" (default) discovers groups of similar memories; \"merge\" applies merge operations.",
					"enum":        []string{"find", "merge"},
				},
				"tags": map[string]any{
					"type":        "array",
					"description": "For find action: only consider memories matching these tags. If omitted, scans all memories.",
					"items":       map[string]any{"type": "string"},
				},
				"keyword": map[string]any{
					"type":        "string",
					"description": "For find action: only consider memories matching this keyword in title or content.",
				},
				"merge_groups": map[string]any{
					"type":        "array",
					"description": "For merge action: array of merge operations to perform.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"source_ids": map[string]any{
								"type":        "array",
								"description": "IDs of memories to merge (will be deleted).",
								"items":       map[string]any{"type": "string"},
							},
							"title": map[string]any{
								"type":        "string",
								"description": "Title for the merged memory.",
							},
							"tags": map[string]any{
								"type":        "array",
								"description": "Tags for the merged memory.",
								"items":       map[string]any{"type": "string"},
							},
							"content": map[string]any{
								"type":        "string",
								"description": "Content for the merged memory.",
							},
							"essential": map[string]any{
								"type":        "boolean",
								"description": "Whether the merged memory is essential.",
							},
						},
						"required": []string{"source_ids", "title", "tags", "content"},
					},
				},
			},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Action      string       `json:"action"`
				Tags        []string     `json:"tags"`
				Keyword     string       `json:"keyword"`
				MergeGroups []mergeGroup `json:"merge_groups"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.Action == "" {
				params.Action = "find"
			}

			switch params.Action {
			case "find":
				return consolidateFind(cfg, params.Tags, params.Keyword)
			case "merge":
				return consolidateMerge(cfg, params.MergeGroups)
			default:
				return "", fmt.Errorf("unknown action: %s (use \"find\" or \"merge\")", params.Action)
			}
		},
	}
}

func consolidateFind(cfg MemoryConfig, filterTags []string, keyword string) (string, error) {
	metas, bodies, err := loadAllMemories(cfg.Dir)
	if err != nil {
		return "", fmt.Errorf("failed to load memories: %w", err)
	}
	if len(metas) == 0 {
		return "No memories found.", nil
	}

	// Filter if tags or keyword provided.
	if len(filterTags) > 0 || keyword != "" {
		var filtered []memoryMeta
		for _, m := range metas {
			matched := false

			if len(filterTags) > 0 {
				tagSet := make(map[string]bool)
				for _, t := range m.Tags {
					tagSet[strings.ToLower(t)] = true
				}
				for _, t := range filterTags {
					if tagSet[strings.ToLower(t)] {
						matched = true
						break
					}
				}
			}

			if keyword != "" && !matched {
				kw := strings.ToLower(keyword)
				if strings.Contains(strings.ToLower(m.Title), kw) {
					matched = true
				}
				if body, ok := bodies[m.ID]; ok && !matched {
					if strings.Contains(strings.ToLower(body), kw) {
						matched = true
					}
				}
			}

			if matched {
				filtered = append(filtered, m)
			}
		}
		metas = filtered
	}

	if len(metas) < 2 {
		return "Not enough memories to find duplicates.", nil
	}

	groups := groupSimilar(metas, bodies, similarityThreshold)
	if len(groups) == 0 {
		return "No similar memory groups found.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d group(s) of similar memories:\n", len(groups))

	for gi, group := range groups {
		fmt.Fprintf(&sb, "\n=== Group %d (%d memories) ===\n", gi+1, len(group))
		for _, idx := range group {
			m := metas[idx]
			body := bodies[m.ID]
			fmt.Fprintf(&sb, "\n- ID: %s\n", m.ID)
			fmt.Fprintf(&sb, "  Title: %s\n", m.Title)
			if len(m.Tags) > 0 {
				fmt.Fprintf(&sb, "  Tags: %s\n", strings.Join(m.Tags, ", "))
			}
			fmt.Fprintf(&sb, "  Essential: %v\n", m.Essential)
			fmt.Fprintf(&sb, "  Created: %s\n", m.Created)
			fmt.Fprintf(&sb, "  Content: %s\n", body)
		}
	}

	sb.WriteString("\nUse action \"merge\" with merge_groups to combine these.")
	return sb.String(), nil
}

type mergeGroup struct {
	SourceIDs []string `json:"source_ids"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	Content   string   `json:"content"`
	Essential bool     `json:"essential"`
}

func consolidateMerge(cfg MemoryConfig, groups []mergeGroup) (string, error) {
	if len(groups) == 0 {
		return "", fmt.Errorf("merge_groups is required for merge action")
	}

	// Validate all source IDs exist before making any changes.
	for gi, g := range groups {
		if len(g.SourceIDs) < 2 {
			return "", fmt.Errorf("group %d: need at least 2 source_ids to merge", gi+1)
		}
		if g.Title == "" {
			return "", fmt.Errorf("group %d: title is required", gi+1)
		}
		if g.Content == "" {
			return "", fmt.Errorf("group %d: content is required", gi+1)
		}
		for _, id := range g.SourceIDs {
			path := filepath.Join(cfg.Dir, id+".md")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return "", fmt.Errorf("group %d: memory not found: %s", gi+1, id)
			}
		}
	}

	var results []string

	for _, g := range groups {
		// Create new merged memory.
		newID := randomID()
		meta := memoryMeta{
			ID:        newID,
			Title:     g.Title,
			Tags:      g.Tags,
			Essential: g.Essential,
			Created:   time.Now().UTC().Format(time.RFC3339),
		}

		if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create memory directory: %w", err)
		}

		fileContent := renderMemoryFile(meta, g.Content)
		filePath := filepath.Join(cfg.Dir, newID+".md")
		if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
			return "", fmt.Errorf("failed to write merged memory: %w", err)
		}

		// Delete source memories.
		for _, id := range g.SourceIDs {
			path := filepath.Join(cfg.Dir, id+".md")
			if err := os.Remove(path); err != nil {
				return "", fmt.Errorf("failed to remove source memory %s: %w", id, err)
			}
		}

		results = append(results, fmt.Sprintf("Merged %d memories into %s, removed [%s]",
			len(g.SourceIDs), newID, strings.Join(g.SourceIDs, ", ")))
	}

	return strings.Join(results, "\n"), nil
}
