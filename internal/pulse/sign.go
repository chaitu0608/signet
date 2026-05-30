package pulse

import (
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// VerifyPushSignature validates EIP-712 push signature when signer and signature present.
func VerifyPushSignature(e Event) error {
	if e.Signer == "" && e.Signature == "" {
		return nil
	}
	if e.Signer == "" || e.Signature == "" {
		return fmt.Errorf("signer and signature required together")
	}

	anchorAddr := os.Getenv("ANCHOR_ADDR")
	if anchorAddr == "" {
		anchorAddr = "0x0000000000000000000000000000000000000000"
	}
	chainID := int64(84532)
	if s := os.Getenv("CHAIN_ID"); s != "" {
		_, _ = fmt.Sscan(s, &chainID)
	}

	oldBytes := padHex32(e.OldSHA)
	newBytes := padHex32(e.NewSHA)

	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"PushEvent": {
				{Name: "repo", Type: "string"},
				{Name: "ref", Type: "string"},
				{Name: "oldSha", Type: "bytes32"},
				{Name: "newSha", Type: "bytes32"},
				{Name: "nonce", Type: "uint64"},
				{Name: "chainTimestamp", Type: "uint64"},
			},
		},
		PrimaryType: "PushEvent",
		Domain: apitypes.TypedDataDomain{
			Name:              "ForgePulse",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(chainID),
			VerifyingContract: anchorAddr,
		},
		Message: apitypes.TypedDataMessage{
			"repo":           e.Repo,
			"ref":            e.Ref,
			"oldSha":         oldBytes,
			"newSha":         newBytes,
			"nonce":          fmt.Sprintf("%d", e.TS.UnixNano()),
			"chainTimestamp": fmt.Sprintf("%d", e.TS.Unix()),
		},
	}

	hash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return err
	}

	sig := strings.TrimPrefix(e.Signature, "0x")
	sigBytes := common.FromHex(sig)
	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length")
	}
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	pub, err := crypto.SigToPub(hash, sigBytes)
	if err != nil {
		return err
	}
	recovered := crypto.PubkeyToAddress(*pub)
	if !strings.EqualFold(recovered.Hex(), e.Signer) {
		return fmt.Errorf("signature mismatch: got %s want %s", recovered.Hex(), e.Signer)
	}
	return nil
}

func padHex32(short string) [32]byte {
	var out [32]byte
	if short == "" {
		return out
	}
	h := common.HexToHash(short)
	copy(out[:], h.Bytes())
	return out
}
