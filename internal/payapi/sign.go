package payapi

import (
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const reportSchema = "SignetReputationReport/v1"

func chainIDFromEnv() int64 {
	s := os.Getenv("CHAIN_ID")
	if s == "" {
		return 84532
	}
	var id int64
	_, _ = fmt.Sscan(s, &id)
	if id == 0 {
		return 84532
	}
	return id
}

func verifyingContract() string {
	if v := os.Getenv("ORACLE_ADDR"); v != "" {
		return v
	}
	if v := os.Getenv("ANCHOR_ADDR"); v != "" {
		return v
	}
	return "0x0000000000000000000000000000000000000000"
}

func reputationTypedData(report ReputationReport) apitypes.TypedData {
	return apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"ReputationReport": {
				{Name: "address", Type: "address"},
				{Name: "totalScore", Type: "uint256"},
				{Name: "commitCount", Type: "uint256"},
				{Name: "weeklyDelta", Type: "int256"},
				{Name: "reportHash", Type: "bytes32"},
				{Name: "issuedAt", Type: "string"},
				{Name: "kind", Type: "string"},
			},
		},
		PrimaryType: "ReputationReport",
		Domain: apitypes.TypedDataDomain{
			Name:              "Signet",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(chainIDFromEnv()),
			VerifyingContract: verifyingContract(),
		},
		Message: apitypes.TypedDataMessage{
			"address":     common.HexToAddress(report.Address).Hex(),
			"totalScore":  fmt.Sprintf("%d", report.TotalScore),
			"commitCount": fmt.Sprintf("%d", report.CommitCount),
			"weeklyDelta": fmt.Sprintf("%d", report.WeeklyDelta),
			"reportHash":  padHex32(report.ReportHash),
			"issuedAt":    report.IssuedAt,
			"kind":        report.Kind,
		},
	}
}

func leaderboardTypedData(report LeaderboardReport) apitypes.TypedData {
	return apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"LeaderboardReport": {
				{Name: "reportHash", Type: "bytes32"},
				{Name: "issuedAt", Type: "string"},
				{Name: "entryCount", Type: "uint256"},
			},
		},
		PrimaryType: "LeaderboardReport",
		Domain: apitypes.TypedDataDomain{
			Name:              "Signet",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(chainIDFromEnv()),
			VerifyingContract: verifyingContract(),
		},
		Message: apitypes.TypedDataMessage{
			"reportHash": padHex32(report.ReportHash),
			"issuedAt":   report.IssuedAt,
			"entryCount": fmt.Sprintf("%d", len(report.Entries)),
		},
	}
}

// SignReputationReport signs a reputation report with the oracle key.
func SignReputationReport(report ReputationReport, privKeyHex string) (signature string, signer string, err error) {
	return signTyped(reputationTypedData(report), privKeyHex)
}

// VerifyReputationReport validates an oracle signature over a reputation report.
func VerifyReputationReport(report ReputationReport, signature, signer string) error {
	return verifyTyped(reputationTypedData(report), signature, signer)
}

// SignLeaderboardReport signs a leaderboard snapshot.
func SignLeaderboardReport(report LeaderboardReport, privKeyHex string) (signature string, signer string, err error) {
	return signTyped(leaderboardTypedData(report), privKeyHex)
}

// VerifyLeaderboardReport validates a leaderboard signature.
func VerifyLeaderboardReport(report LeaderboardReport, signature, signer string) error {
	return verifyTyped(leaderboardTypedData(report), signature, signer)
}

func signTyped(typed apitypes.TypedData, privKeyHex string) (string, string, error) {
	if privKeyHex == "" {
		return "", "", fmt.Errorf("oracle private key not configured")
	}
	priv, err := crypto.HexToECDSA(strings.TrimPrefix(privKeyHex, "0x"))
	if err != nil {
		return "", "", err
	}
	hash, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		return "", "", err
	}
	sig, err := crypto.Sign(hash, priv)
	if err != nil {
		return "", "", err
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	signerAddr := crypto.PubkeyToAddress(priv.PublicKey)
	return "0x" + common.Bytes2Hex(sig), signerAddr.Hex(), nil
}

func verifyTyped(typed apitypes.TypedData, signature, signer string) error {
	hash, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		return err
	}
	sig := strings.TrimPrefix(signature, "0x")
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
	if !strings.EqualFold(recovered.Hex(), signer) {
		return fmt.Errorf("signature mismatch: got %s want %s", recovered.Hex(), signer)
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
