package pulse

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/crypto"
)

// EventLeafHash computes keccak256 of canonical event fields for anchoring.
func EventLeafHash(e Event) (string, error) {
	canonical := struct {
		ID     string `json:"id"`
		Repo   string `json:"repo"`
		Ref    string `json:"ref"`
		OldSHA string `json:"old_sha"`
		NewSHA string `json:"new_sha"`
		Pusher string `json:"pusher"`
		Signer string `json:"signer"`
		Type   string `json:"type"`
		TS     int64  `json:"ts_unix"`
	}{
		ID:     e.ID,
		Repo:   e.Repo,
		Ref:    e.Ref,
		OldSHA: e.OldSHA,
		NewSHA: e.NewSHA,
		Pusher: e.Pusher,
		Signer: e.Signer,
		Type:   e.Type,
		TS:     e.TS.Unix(),
	}
	b, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return crypto.Keccak256Hash(b).Hex(), nil
}
