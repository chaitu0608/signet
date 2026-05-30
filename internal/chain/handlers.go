package chain

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"server/internal/store"
)

// HandleProof returns Merkle proof for an event.
func HandleProof(st store.Store, cli *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/proof/")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		proof, err := st.GetProof(r.Context(), id)
		if err != nil || proof == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		verified := false
		if proof.Root != "" {
			verified = VerifyProof(proof.Root, proof.LeafHash, proof.Proof)
		}
		if cli != nil && proof.Root != "" && len(proof.Proof) > 0 {
			var leaf [32]byte
			copy(leaf[:], common.HexToHash(proof.LeafHash).Bytes())
			var siblings [][32]byte
			for _, p := range proof.Proof {
				var s [32]byte
				copy(s[:], common.HexToHash(p).Bytes())
				siblings = append(siblings, s)
			}
			onChain, err := cli.VerifyOnChain(r.Context(), proof.BatchID, leaf, siblings)
			if err == nil && onChain {
				verified = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proof":    proof,
			"verified": verified,
		})
	}
}

// BountyClaimRequest is the claim API body.
type BountyClaimRequest struct {
	BountyID uint64 `json:"bounty_id"`
	Payee    string `json:"payee"`
}

// HandleBountyClaim signs oracle claim for on-chain bounty.
func HandleBountyClaim(st store.Store, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req BountyClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sig, err := SignOracleClaim(cfg.PrivKey, req.BountyID, common.HexToAddress(req.Payee), common.HexToAddress(cfg.Bounty), cfg.ChainID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"oracle_sig": "0x" + common.Bytes2Hex(sig),
			"bounty_id":  req.BountyID,
			"payee":      req.Payee,
			"escrow":     cfg.Bounty,
		})
	}
}

// HandleBounties lists bounties.
func HandleBounties(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}
		list, err := st.ListBounties(r.Context(), status, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"bounties": list})
	}
}

// HandleSBTLookup returns events for a signer.
func HandleSBTLookup(st store.Store, cli *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addr := strings.TrimPrefix(r.URL.Path, "/api/sbt/")
		if addr == "" {
			http.Error(w, "missing address", http.StatusBadRequest)
			return
		}
		events, _ := st.EventsBySigner(r.Context(), addr, 20)
		balance := "0"
		if cli != nil && cli.SBT != (common.Address{}) {
			data, err := cli.sbtABI.Pack("balanceOf", common.HexToAddress(addr))
			if err == nil {
				out, err := cli.eth.CallContract(r.Context(), ethereum.CallMsg{To: &cli.SBT, Data: data}, nil)
				if err == nil {
					vals, err := cli.sbtABI.Unpack("balanceOf", out)
					if err == nil && len(vals) > 0 {
						balance = vals[0].(*big.Int).String()
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"address": addr,
			"events":  events,
			"balance": balance,
		})
	}
}

// HandleChainConfig returns public chain addresses for the web3 UI.
func HandleChainConfig(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chain_id":       cfg.ChainID,
			"anchor":         cfg.Anchor,
			"bounty":         cfg.Bounty,
			"sbt":            cfg.SBT,
			"eas":            cfg.EASAddr,
			"eas_schema_uid": cfg.EASSchemaUID,
			"enabled":        cfg.Enabled(),
			"eas_enabled":    cfg.EASEnabled(),
			"explorer":       "https://sepolia.basescan.org",
			"eas_explorer":   "https://base-sepolia.easscan.org",
		})
	}
}

// RegisterBounty mirrors an on-chain fund event into the database.
func RegisterBounty(st store.Store, issueHash, funder, amount string, onChainID uint64) error {
	return st.InsertBounty(context.Background(), store.Bounty{
		IssueHash: issueHash,
		Funder:    funder,
		AmountWei: amount,
		Token:     "ETH",
		Status:    "active",
		OnChainID: onChainID,
	})
}
