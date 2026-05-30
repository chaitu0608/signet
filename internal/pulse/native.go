package pulse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

)

type nativePayload struct {
	Repo          string       `json:"repo"`
	Ref           string       `json:"ref"`
	OldSHA        string       `json:"old_sha"`
	NewSHA        string       `json:"new_sha"`
	Pusher        string       `json:"pusher"`
	Signer        string       `json:"signer"`
	Signature     string       `json:"signature"`
	CommitsDetail []CommitInfo `json:"commits_detail"`
}

// VerifyNativeAuth checks bearer token when HOOK_TOKEN is configured.
func VerifyNativeAuth(r *http.Request, token string) error {
	if token == "" {
		return nil
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) || strings.TrimPrefix(auth, prefix) != token {
		return fmt.Errorf("unauthorized")
	}
	return nil
}

// ParseNativeHook reads a native post-receive JSON payload.
func ParseNativeHook(r *http.Request) (Event, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Event{}, err
	}

	var p nativePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, err
	}
	if p.Repo == "" || p.Ref == "" || p.NewSHA == "" {
		return Event{}, fmt.Errorf("missing required fields")
	}

	e := NewEvent()
	e.TS = time.Now().UTC()
	e.Repo = p.Repo
	e.Ref = p.Ref
	e.Branch = RefBranch(p.Ref)
	e.OldSHA = shortSHA(p.OldSHA)
	e.NewSHA = shortSHA(p.NewSHA)
	e.Pusher = p.Pusher
	e.Signer = p.Signer
	e.Signature = p.Signature
	e.Source = SourceNative
	e.CommitsDetail = p.CommitsDetail
	e.Commits = len(p.CommitsDetail)

	zeroSHA := strings.HasPrefix(p.OldSHA, "0000000") || p.OldSHA == ""
	if zeroSHA {
		if strings.HasPrefix(p.Ref, "refs/tags/") {
			e.Type = TypeTag
		} else {
			e.Type = TypeBranchCreate
		}
	} else {
		e.Type = TypePush
	}

	if err := VerifyPushSignature(e); err != nil {
		return Event{}, err
	}

	leaf, err := EventLeafHash(e)
	if err != nil {
		return Event{}, err
	}
	e.LeafHash = leaf

	return e, nil
}
