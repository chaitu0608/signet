package reviewer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"server/internal/domain"
)

type heuristic struct{}

func (h *heuristic) Review(ctx context.Context, e domain.Event) (Review, error) {
	if !e.IsPushLike() {
		return Review{Score: 50, Summary: "non-push event", Category: "chore"}, nil
	}

	diff := gitDiff(ctx, e.Repo, e.OldSHA, e.NewSHA)
	lines := strings.Count(diff, "\n")
	score := 40
	if lines > 0 && lines < 500 {
		score += 20
	}
	if lines >= 500 && lines < 2000 {
		score += 10
	}

	lower := strings.ToLower(diff + " " + strings.Join(commitMessages(e), " "))
	var security []string
	for _, bad := range []string{"eval(", "exec(", "password=", "private_key", "0xprivate"} {
		if strings.Contains(lower, bad) {
			security = append(security, "suspicious: "+bad)
			score -= 15
		}
	}
	if strings.Contains(lower, "test") || strings.Contains(lower, "_test.") {
		score += 15
	}
	if strings.Contains(lower, "fix") || strings.Contains(lower, "security") {
		score += 10
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	category := inferCategory(lower)
	summary := fmt.Sprintf("heuristic: %d lines changed", lines)
	if len(security) > 0 {
		summary = "heuristic: flagged security patterns"
	}
	return Review{
		Score:    score,
		Summary:  summary,
		Security: security,
		Tests:    strings.Contains(lower, "test"),
		Category: category,
	}, nil
}

func inferCategory(lower string) string {
	switch {
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "fix") || strings.Contains(lower, "bug"):
		return "fix"
	case strings.Contains(lower, "refactor"):
		return "refactor"
	case strings.Contains(lower, "doc") || strings.Contains(lower, "readme"):
		return "docs"
	case strings.Contains(lower, "feat") || strings.Contains(lower, "add "):
		return "feature"
	default:
		return "chore"
	}
}

func commitMessages(e domain.Event) []string {
	var msgs []string
	for _, c := range e.CommitsDetail {
		msgs = append(msgs, c.Message)
	}
	return msgs
}

func gitDiff(ctx context.Context, repo, oldSHA, newSHA string) string {
	if repo == "" || newSHA == "" {
		return ""
	}
	old := oldSHA
	if strings.HasPrefix(old, "0000000") || old == "" {
		old = newSHA + "^"
	}
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "diff", old+".."+newSHA).CombinedOutput()
	if err != nil {
		out2, _ := exec.CommandContext(ctx, "git", "-C", repo, "show", newSHA, "--stat").CombinedOutput()
		return string(out2)
	}
	if len(out) > 8192 {
		return string(out[:8192])
	}
	return string(out)
}
