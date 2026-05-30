package pulse

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func TestVerifyPushSignature_Unsigned(t *testing.T) {
	// no signer + no signature is allowed (backward compat path)
	if err := VerifyPushSignature(Event{}); err != nil {
		t.Fatalf("unsigned should pass: %v", err)
	}
}

func TestVerifyPushSignature_HalfSigned(t *testing.T) {
	if err := VerifyPushSignature(Event{Signer: "0x0000000000000000000000000000000000000001"}); err == nil {
		t.Fatal("signer without signature must fail")
	}
}

func TestVerifyPushSignature_RoundTrip(t *testing.T) {
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.PubkeyToAddress(priv.PublicKey)

	e := Event{
		TS:     time.Unix(1700000000, 0).UTC(),
		Repo:   "test/repo",
		Ref:    "refs/heads/main",
		Branch: "main",
		OldSHA: "0000000000000000000000000000000000000000",
		NewSHA: "abcdefabcdefabcdefabcdefabcdefabcdefabcd",
		Signer: signer.Hex(),
	}

	old32 := padHex32(e.OldSHA)
	new32 := padHex32(e.NewSHA)
	typed := apitypes.TypedData{
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
			ChainId:           math.NewHexOrDecimal256(84532),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: apitypes.TypedDataMessage{
			"repo":           e.Repo,
			"ref":            e.Ref,
			"oldSha":         old32,
			"newSha":         new32,
			"nonce":          fmt.Sprintf("%d", e.TS.UnixNano()),
			"chainTimestamp": fmt.Sprintf("%d", e.TS.Unix()),
		},
	}

	hash, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(hash, priv)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	e.Signature = "0x" + common.Bytes2Hex(sig)

	if err := VerifyPushSignature(e); err != nil {
		t.Fatalf("signed roundtrip should pass: %v", err)
	}

	// tamper -> mismatch
	e.Repo = "tampered/repo"
	if err := VerifyPushSignature(e); err == nil {
		t.Fatal("tampered payload should fail verification")
	}
}
