package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

type ChunkPlan struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DependsOn   int    `json:"depends_on"`
}

func DecomposeTask(ctx context.Context, client Client, description string) ([]ChunkPlan, error) {
	system := `You are a senior engineer. Break the user's task into 3-7 small, implementable chunks.
Return ONLY a JSON array of objects with fields: title (string), description (string), depends_on (0-based index of dependency, or omit for none).`
	resp, err := client.Chat(ctx, system, description)
	if err != nil {
		return nil, err
	}
	// tolerate markdown fences
	resp = stripMarkdown(resp)
	var chunks []ChunkPlan
	if err := json.Unmarshal([]byte(resp), &chunks); err != nil {
		return nil, fmt.Errorf("parse decomposition: %w\nresponse: %s", err, resp)
	}
	for i := range chunks {
		if chunks[i].Title == "" || chunks[i].Description == "" {
			return nil, fmt.Errorf("chunk %d missing title or description", i)
		}
	}
	return chunks, nil
}

func stripMarkdown(s string) string {
	start := 0
	end := len(s)
	if i := indexOf(s, "```json"); i != -1 {
		start = i + len("```json")
		if j := lastIndexOf(s[start:], "```"); j != -1 {
			end = start + j
		}
		return s[start:end]
	}
	return s
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
