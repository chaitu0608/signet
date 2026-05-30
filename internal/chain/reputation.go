package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"server/internal/domain"
)

const reputationABIJSON = `[{"inputs":[{"internalType":"address","name":"agent","type":"address"},{"internalType":"int128","name":"score","type":"int128"},{"internalType":"string","name":"tag","type":"string"},{"internalType":"bytes32","name":"evidenceRoot","type":"bytes32"},{"internalType":"bytes32","name":"eventLeaf","type":"bytes32"}],"name":"giveFeedback","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

// WriteReputation records attested push feedback on the SignetReputation registry.
func (c *Client) WriteReputation(ctx context.Context, e domain.Event, evidenceRoot string) (string, error) {
	if c.Reputation == (common.Address{}) {
		return "", fmt.Errorf("reputation registry not configured")
	}
	repABI, err := abi.JSON(strings.NewReader(reputationABIJSON))
	if err != nil {
		return "", err
	}

	score := int64(e.QualityScore)
	if score > 100 {
		score = 100
	}
	tag := e.Category
	if tag == "" {
		tag = "chore"
	}

	var root [32]byte
	copy(root[:], common.HexToHash(evidenceRoot).Bytes())
	var leaf [32]byte
	copy(leaf[:], common.HexToHash(e.LeafHash).Bytes())

	data, err := repABI.Pack("giveFeedback",
		common.HexToAddress(e.Signer),
		big.NewInt(score),
		tag,
		root,
		leaf,
	)
	if err != nil {
		return "", err
	}

	nonce, err := c.eth.PendingNonceAt(ctx, c.from)
	if err != nil {
		return "", err
	}
	gasPrice, err := c.eth.SuggestGasPrice(ctx)
	if err != nil {
		return "", err
	}

	tx := types.NewTransaction(nonce, c.Reputation, big.NewInt(0), 300_000, gasPrice, data)
	signed, err := types.SignTx(tx, types.NewEIP155Signer(c.chainID), c.relayer)
	if err != nil {
		return "", err
	}
	if err := c.eth.SendTransaction(ctx, signed); err != nil {
		return "", err
	}
	_, err = bind.WaitMined(ctx, c.eth, signed)
	if err != nil {
		return signed.Hash().Hex(), err
	}
	return signed.Hash().Hex(), nil
}
