package chain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client wraps chain RPC and relayer signing.
type Client struct {
	eth     *ethclient.Client
	relayer *ecdsa.PrivateKey
	from    common.Address
	chainID *big.Int

	Anchor         common.Address
	Bounty         common.Address
	SBT            common.Address
	EAS            common.Address
	SchemaRegistry common.Address
	SchemaUID      common.Hash

	anchorABI abi.ABI
	bountyABI abi.ABI
	sbtABI    abi.ABI
	easABI    abi.ABI
}

// Config from environment.
type Config struct {
	RPCURL  string
	ChainID int64
	PrivKey string

	Anchor string
	Bounty string
	SBT    string

	EASAddr           string
	EASSchemaRegistry string
	EASSchemaUID      string
}

// LoadConfig reads chain config from env.
func LoadConfig() Config {
	return Config{
		RPCURL:            os.Getenv("RPC_URL"),
		ChainID:           chainIDFromEnv(),
		PrivKey:           os.Getenv("RELAYER_PRIVATE_KEY"),
		Anchor:            os.Getenv("ANCHOR_ADDR"),
		Bounty:            os.Getenv("BOUNTY_ADDR"),
		SBT:               os.Getenv("SBT_ADDR"),
		EASAddr:           easAddrFromEnv(),
		EASSchemaRegistry: schemaRegFromEnv(),
		EASSchemaUID:      os.Getenv("EAS_SCHEMA_UID"),
	}
}

// Enabled returns true if chain client can operate.
func (c Config) Enabled() bool {
	return c.RPCURL != "" && c.PrivKey != "" && c.Anchor != ""
}

// NewClient connects to RPC and parses ABIs.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	eth, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, err
	}
	priv, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivKey, "0x"))
	if err != nil {
		return nil, err
	}

	anchorABI, err := abi.JSON(strings.NewReader(anchorABIJSON))
	if err != nil {
		return nil, err
	}
	bountyABI, err := abi.JSON(strings.NewReader(bountyABIJSON))
	if err != nil {
		return nil, err
	}
	sbtABI, err := abi.JSON(strings.NewReader(sbtABIJSON))
	if err != nil {
		return nil, err
	}
	easABI, err := abi.JSON(strings.NewReader(easABIJSON))
	if err != nil {
		return nil, err
	}

	var schemaUID common.Hash
	if cfg.EASSchemaUID != "" {
		schemaUID = common.HexToHash(cfg.EASSchemaUID)
	}

	return &Client{
		eth:            eth,
		relayer:        priv,
		from:           crypto.PubkeyToAddress(priv.PublicKey),
		chainID:        big.NewInt(cfg.ChainID),
		Anchor:         common.HexToAddress(cfg.Anchor),
		Bounty:         common.HexToAddress(cfg.Bounty),
		SBT:            common.HexToAddress(cfg.SBT),
		EAS:            common.HexToAddress(cfg.EASAddr),
		SchemaRegistry: common.HexToAddress(cfg.EASSchemaRegistry),
		SchemaUID:      schemaUID,
		anchorABI:      anchorABI,
		bountyABI:      bountyABI,
		sbtABI:         sbtABI,
		easABI:         easABI,
	}, nil
}

func (c *Client) transactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	nonce, err := c.eth.PendingNonceAt(ctx, c.from)
	if err != nil {
		return nil, err
	}
	gasPrice, err := c.eth.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	return &bind.TransactOpts{
		From:     c.from,
		Nonce:    big.NewInt(int64(nonce)),
		Signer:   c.signer,
		GasPrice: gasPrice,
		GasLimit: 800000,
		Context:  ctx,
	}, nil
}

func (c *Client) signer(_ common.Address, tx *types.Transaction) (*types.Transaction, error) {
	return types.SignTx(tx, types.NewEIP155Signer(c.chainID), c.relayer)
}

func (c *Client) send(_ context.Context, opts *bind.TransactOpts, to common.Address, data []byte) (*types.Transaction, error) {
	value := big.NewInt(0)
	tx := types.NewTransaction(opts.Nonce.Uint64(), to, value, opts.GasLimit, opts.GasPrice, data)
	return opts.Signer(opts.From, tx)
}

// AnchorRoot submits anchor(root, leaves) transaction.
func (c *Client) AnchorRoot(ctx context.Context, root [32]byte, leaves uint32) (txHash string, err error) {
	opts, err := c.transactOpts(ctx)
	if err != nil {
		return "", err
	}
	data, err := c.anchorABI.Pack("anchor", root, leaves)
	if err != nil {
		return "", err
	}
	tx, err := c.send(ctx, opts, c.Anchor, data)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, c.eth, tx)
	if err != nil {
		return tx.Hash().Hex(), err
	}
	if receipt.Status == 0 {
		return tx.Hash().Hex(), fmt.Errorf("anchor tx reverted")
	}
	return tx.Hash().Hex(), nil
}

// BatchCount reads on-chain batch count.
func (c *Client) BatchCount(ctx context.Context) (uint64, error) {
	data, err := c.anchorABI.Pack("batchCount")
	if err != nil {
		return 0, err
	}
	out, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.Anchor, Data: data}, nil)
	if err != nil {
		return 0, err
	}
	vals, err := c.anchorABI.Unpack("batchCount", out)
	if err != nil || len(vals) == 0 {
		return 0, err
	}
	return vals[0].(*big.Int).Uint64(), nil
}

// MintSBTBatch calls mintBatch on ContribSBT.
func (c *Client) MintSBTBatch(ctx context.Context, devs []common.Address, hashes [][32]byte) (string, error) {
	if c.SBT == (common.Address{}) {
		return "", fmt.Errorf("SBT_ADDR not set")
	}
	opts, err := c.transactOpts(ctx)
	if err != nil {
		return "", err
	}
	data, err := c.sbtABI.Pack("mintBatch", devs, hashes)
	if err != nil {
		return "", err
	}
	tx, err := c.send(ctx, opts, c.SBT, data)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, c.eth, tx)
	if err != nil {
		return tx.Hash().Hex(), err
	}
	if receipt.Status == 0 {
		return tx.Hash().Hex(), fmt.Errorf("mint tx reverted")
	}
	return tx.Hash().Hex(), nil
}

// VerifyOnChain calls anchor.verify view.
func (c *Client) VerifyOnChain(ctx context.Context, batchID uint64, leaf [32]byte, proof [][32]byte) (bool, error) {
	data, err := c.anchorABI.Pack("verify", big.NewInt(int64(batchID)), leaf, proof)
	if err != nil {
		return false, err
	}
	out, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.Anchor, Data: data}, nil)
	if err != nil {
		return false, err
	}
	vals, err := c.anchorABI.Unpack("verify", out)
	if err != nil || len(vals) == 0 {
		return false, err
	}
	return vals[0].(bool), nil
}

const anchorABIJSON = `[{"inputs":[{"internalType":"bytes32","name":"root","type":"bytes32"},{"internalType":"uint32","name":"leaves","type":"uint32"}],"name":"anchor","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[],"name":"batchCount","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"batchId","type":"uint256"},{"internalType":"bytes32","name":"leaf","type":"bytes32"},{"internalType":"bytes32[]","name":"proof","type":"bytes32[]"}],"name":"verify","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"view","type":"function"}]`

const bountyABIJSON = `[{"inputs":[{"internalType":"uint256","name":"bountyId","type":"uint256"},{"internalType":"address","name":"payee","type":"address"},{"internalType":"bytes","name":"oracleSig","type":"bytes"}],"name":"claim","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

const sbtABIJSON = `[{"inputs":[{"internalType":"address[]","name":"devs","type":"address[]"},{"internalType":"bytes32[]","name":"hashes","type":"bytes32[]"}],"name":"mintBatch","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`
