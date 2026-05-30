package chain

import (
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

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

// SignOracleClaim signs bounty claim digest for BountyEscrow.claim.
func SignOracleClaim(privKeyHex string, bountyID uint64, payee common.Address, escrow common.Address, chainID int64) ([]byte, error) {
	priv, err := crypto.HexToECDSA(strings.TrimPrefix(privKeyHex, "0x"))
	if err != nil {
		return nil, err
	}
	digest := crypto.Keccak256(
		common.LeftPadBytes(big.NewInt(int64(bountyID)).Bytes(), 32),
		common.LeftPadBytes(payee.Bytes(), 32),
		common.LeftPadBytes(escrow.Bytes(), 32),
		common.LeftPadBytes(big.NewInt(chainID).Bytes(), 32),
	)
	prefixed := crypto.Keccak256(
		[]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d", 32)),
		digest,
	)
	sig, err := crypto.Sign(prefixed, priv)
	if err != nil {
		return nil, err
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}
