package pulse

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

)

type githubPusher struct {
	Name string `json:"name"`
}

type githubRepo struct {
	FullName string `json:"full_name"`
}

type githubCommit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type githubPushPayload struct {
	Ref        string          `json:"ref"`
	Before     string          `json:"before"`
	After      string          `json:"after"`
	Forced     bool            `json:"forced"`
	Pusher     githubPusher    `json:"pusher"`
	Repository githubRepo      `json:"repository"`
	Commits    []githubCommit  `json:"commits"`
	HeadCommit *githubCommit   `json:"head_commit"`
}

type githubRefPayload struct {
	Ref        string       `json:"ref"`
	RefType    string       `json:"ref_type"`
	Repository githubRepo   `json:"repository"`
	Sender     githubPusher `json:"sender"`
}

// VerifyGitHubSignature checks X-Hub-Signature-256 when secret is set.
func VerifyGitHubSignature(secret string, body []byte, signature string) error {
	if secret == "" {
		return nil
	}
	if signature == "" {
		return fmt.Errorf("missing signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func commitsFromGitHub(commits []githubCommit) []CommitInfo {
	out := make([]CommitInfo, 0, len(commits))
	for _, c := range commits {
		msg := c.Message
		if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
			msg = msg[:idx]
		}
		out = append(out, CommitInfo{
			SHA:     shortSHA(c.ID),
			Message: strings.TrimSpace(msg),
		})
	}
	return out
}

func githubLagMS(deliveryTime time.Time, head *githubCommit) int64 {
	if head == nil || head.Timestamp == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, head.Timestamp)
	if err != nil {
		return 0
	}
	lag := deliveryTime.Sub(t).Milliseconds()
	if lag < 0 {
		return 0
	}
	return lag
}

// ParseGitHubWebhook converts a GitHub delivery into events.
func ParseGitHubWebhook(eventType string, body []byte, deliveryTime time.Time) ([]Event, error) {
	switch eventType {
	case "push":
		var p githubPushPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		e := NewEvent()
		e.TS = deliveryTime
		e.Repo = p.Repository.FullName
		e.Ref = p.Ref
		e.Branch = RefBranch(p.Ref)
		e.OldSHA = shortSHA(p.Before)
		e.NewSHA = shortSHA(p.After)
		e.Pusher = p.Pusher.Name
		e.Commits = len(p.Commits)
		e.Force = p.Forced
		e.Source = SourceGitHub
		e.GitHubLagMS = githubLagMS(deliveryTime, p.HeadCommit)
		e.CommitsDetail = commitsFromGitHub(p.Commits)
		if p.Forced {
			e.Type = TypeForcePush
		} else {
			e.Type = TypePush
		}
		if h, err := EventLeafHash(e); err == nil {
			e.LeafHash = h
		}
		return []Event{e}, nil

	case "create":
		var p githubRefPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		e := NewEvent()
		e.TS = deliveryTime
		e.Repo = p.Repository.FullName
		e.Ref = p.Ref
		e.Branch = RefBranch(p.Ref)
		e.Pusher = p.Sender.Name
		e.Source = SourceGitHub
		if p.RefType == "tag" {
			e.Type = TypeTag
		} else {
			e.Type = TypeBranchCreate
		}
		return []Event{e}, nil

	case "delete":
		var p githubRefPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		e := NewEvent()
		e.TS = deliveryTime
		e.Repo = p.Repository.FullName
		e.Ref = p.Ref
		e.Branch = RefBranch(p.Ref)
		e.Pusher = p.Sender.Name
		e.Source = SourceGitHub
		if p.RefType == "tag" {
			e.Type = TypeTag
		} else {
			e.Type = TypeBranchDelete
		}
		return []Event{e}, nil

	default:
		return nil, fmt.Errorf("unsupported event: %s", eventType)
	}
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ReadGitHubHook reads and validates a GitHub webhook request.
func ReadGitHubHook(r *http.Request, secret string) ([]Event, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := VerifyGitHubSignature(secret, body, sig); err != nil {
		return nil, err
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		return nil, fmt.Errorf("missing X-GitHub-Event")
	}

	if strings.Contains(eventType, ",") {
		eventType = strings.Split(eventType, ",")[0]
	}

	return ParseGitHubWebhook(eventType, body, time.Now().UTC())
}
