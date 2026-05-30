package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"server/internal/domain"
)

type openAI struct {
	apiKey string
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat *respFmt      `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respFmt struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

var validCategories = map[string]bool{
	"feature": true, "fix": true, "refactor": true, "docs": true, "test": true, "chore": true,
}

func (o *openAI) Review(ctx context.Context, e domain.Event) (Review, error) {
	if !e.IsPushLike() {
		return (&heuristic{}).Review(ctx, e)
	}

	diff := gitDiff(ctx, e.Repo, e.OldSHA, e.NewSHA)
	prompt := fmt.Sprintf(`Grade this git push for portable on-chain developer reputation. Your one-line summary will be public and permanent.

Repo: %s branch: %s pusher: %s
Commits: %v
Diff (truncated):
%s

Respond ONLY with JSON:
{"score":0-100,"summary":"one public line","security":["concerns or empty"],"adds_tests":true/false,"category":"feature|fix|refactor|docs|test|chore"}`,
		e.Repo, e.Branch, e.Pusher, e.CommitsDetail, diff)

	body, _ := json.Marshal(chatRequest{
		Model: "gpt-4o-mini",
		Messages: []chatMessage{
			{Role: "system", Content: "You grade developer commits for permanent on-chain reputation. Be fair, concise, security-aware. Output JSON only."},
			{Role: "user", Content: prompt},
		},
		ResponseFormat: &respFmt{Type: "json_object"},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return (&heuristic{}).Review(ctx, e)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return (&heuristic{}).Review(ctx, e)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return (&heuristic{}).Review(ctx, e)
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return (&heuristic{}).Review(ctx, e)
	}
	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	var review Review
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return (&heuristic{}).Review(ctx, e)
	}
	if review.Score < 0 {
		review.Score = 0
	}
	if review.Score > 100 {
		review.Score = 100
	}
	if review.Summary == "" {
		review.Summary = "AI review complete"
	}
	if !validCategories[review.Category] {
		review.Category = inferCategory(strings.ToLower(review.Summary))
	}
	return review, nil
}
