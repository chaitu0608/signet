package chain

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"server/internal/domain"
)

const (
	defaultEASAddr   = "0x4200000000000000000000000000000000000021"
	defaultSchemaReg = "0x4200000000000000000000000000000000000020"
	forgeSchemaString = "bytes32 commitHash,bytes32 leafHash,string repo,string branch,uint16 qualityScore,string aiSummary,string[] securityFlags,string category"
)

const easABIJSON = `[{"inputs":[{"components":[{"internalType":"bytes32","name":"schema","type":"bytes32"},{"components":[{"internalType":"address","name":"recipient","type":"address"},{"internalType":"uint64","name":"expirationTime","type":"uint64"},{"internalType":"bool","name":"revocable","type":"bool"},{"internalType":"bytes32","name":"refUID","type":"bytes32"},{"internalType":"bytes","name":"data","type":"bytes"},{"internalType":"uint256","name":"value","type":"uint256"}],"internalType":"struct AttestationRequestData","name":"data","type":"tuple"}],"internalType":"struct AttestationRequest","name":"request","type":"tuple"}],"name":"attest","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"payable","type":"function"}]`

const schemaRegistryABIJSON = `[{"inputs":[{"internalType":"string","name":"schema","type":"string"},{"internalType":"address","name":"resolver","type":"address"},{"internalType":"bool","name":"revocable","type":"bool"}],"name":"register","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"nonpayable","type":"function"}]`

type attestationRequestData struct {
	Recipient      common.Address `abi:"recipient"`
	ExpirationTime uint64         `abi:"expirationTime"`
	Revocable      bool           `abi:"revocable"`
	RefUID         [32]byte       `abi:"refUID"`
	Data           []byte         `abi:"data"`
	Value          *big.Int       `abi:"value"`
}

type attestationRequest struct {
	Schema [32]byte               `abi:"schema"`
	Data   attestationRequestData `abi:"data"`
}

var schemaDataArgs abi.Arguments

func init() {
	schemaDataArgs = abi.Arguments{
		{Type: mustABIType("bytes32"), Name: "commitHash"},
		{Type: mustABIType("bytes32"), Name: "leafHash"},
		{Type: mustABIType("string"), Name: "repo"},
		{Type: mustABIType("string"), Name: "branch"},
		{Type: mustABIType("uint16"), Name: "qualityScore"},
		{Type: mustABIType("string"), Name: "aiSummary"},
		{Type: mustABIType("string[]"), Name: "securityFlags"},
		{Type: mustABIType("string"), Name: "category"},
	}
}

func mustABIType(t string) abi.Type {
	ty, err := abi.NewType(t, "", nil)
	if err != nil {
		panic(err)
	}
	return ty
}

func easAddrFromEnv() string {
	if v := os.Getenv("EAS_ADDR"); v != "" {
		return v
	}
	return defaultEASAddr
}

func schemaRegFromEnv() string {
	if v := os.Getenv("EAS_SCHEMA_REGISTRY"); v != "" {
		return v
	}
	return defaultSchemaReg
}

// EASEnabled is true when schema UID and relayer are configured.
func (c Config) EASEnabled() bool {
	return c.Enabled() && c.EASSchemaUID != ""
}

// Attest posts a canonical EAS attestation for a signed, anchored push.
func (c *Client) Attest(ctx context.Context, e domain.Event) (uid common.Hash, txHash string, err error) {
	if c.EAS == (common.Address{}) || c.SchemaUID == (common.Hash{}) {
		return common.Hash{}, "", fmt.Errorf("EAS not configured (set EAS_SCHEMA_UID)")
	}
	recipient := common.HexToAddress(e.Signer)
	if recipient == (common.Address{}) {
		return common.Hash{}, "", fmt.Errorf("no signer")
	}

	encoded, err := encodeSchemaData(e)
	if err != nil {
		return common.Hash{}, "", err
	}

	opts, err := c.transactOpts(ctx)
	if err != nil {
		return common.Hash{}, "", err
	}

	req := attestationRequest{
		Schema: c.SchemaUID,
		Data: attestationRequestData{
			Recipient:      recipient,
			ExpirationTime: 0,
			Revocable:      true,
			RefUID:         [32]byte{},
			Data:           encoded,
			Value:          big.NewInt(0),
		},
	}

	data, err := c.easABI.Pack("attest", req)
	if err != nil {
		return common.Hash{}, "", err
	}

	tx, err := c.send(ctx, opts, c.EAS, data)
	if err != nil {
		return common.Hash{}, "", err
	}

	receipt, err := bind.WaitMined(ctx, c.eth, tx)
	if err != nil {
		return common.Hash{}, tx.Hash().Hex(), err
	}
	if receipt.Status == 0 {
		return common.Hash{}, tx.Hash().Hex(), fmt.Errorf("EAS attest reverted")
	}

	uid = parseAttestedUID(receipt.Logs)
	return uid, tx.Hash().Hex(), nil
}

func encodeSchemaData(e domain.Event) ([]byte, error) {
	commitHash := common.HexToHash(e.NewSHA)
	if commitHash == (common.Hash{}) {
		commitHash = crypto.Keccak256Hash([]byte(e.ID + e.NewSHA))
	}
	leafHash := common.HexToHash(e.LeafHash)
	score := uint16(e.QualityScore)
	if score == 0 {
		score = 50
	}
	summary := e.QualitySummary
	if summary == "" {
		summary = "forge push"
	}
	category := e.Category
	if category == "" {
		category = "chore"
	}
	flags := e.SecurityFlags
	if flags == nil {
		flags = []string{}
	}
	return schemaDataArgs.Pack(commitHash, leafHash, e.Repo, e.Branch, score, summary, flags, category)
}

func parseAttestedUID(logs []*types.Log) common.Hash {
	topic := crypto.Keccak256Hash([]byte("Attested(address,address,bytes32,bytes32)"))
	for _, lg := range logs {
		if len(lg.Topics) > 0 && lg.Topics[0] == topic && len(lg.Data) >= 32 {
			return common.BytesToHash(lg.Data[:32])
		}
	}
	return common.Hash{}
}

// RegisterSchema registers the Forge schema on EAS SchemaRegistry (one-time).
func (c *Client) RegisterSchema(ctx context.Context) (common.Hash, error) {
	if c.SchemaRegistry == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("schema registry not set")
	}
	regABI, err := abi.JSON(strings.NewReader(schemaRegistryABIJSON))
	if err != nil {
		return common.Hash{}, err
	}
	opts, err := c.transactOpts(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	data, err := regABI.Pack("register", forgeSchemaString, common.Address{}, true)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := c.send(ctx, opts, c.SchemaRegistry, data)
	if err != nil {
		return common.Hash{}, err
	}
	receipt, err := bind.WaitMined(ctx, c.eth, tx)
	if err != nil {
		return common.Hash{}, err
	}
	if receipt.Status == 0 {
		return common.Hash{}, fmt.Errorf("register schema reverted")
	}
	regTopic := crypto.Keccak256Hash([]byte("Registered(bytes32,address)"))
	for _, lg := range receipt.Logs {
		if len(lg.Topics) >= 2 && lg.Topics[0] == regTopic {
			return lg.Topics[1], nil
		}
	}
	return common.Hash{}, fmt.Errorf("schema uid not found in logs")
}
