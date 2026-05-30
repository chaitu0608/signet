package payapi

import (
	"os"
	"strings"

	"github.com/mark3labs/x402-go"
)

const (
	defaultFacilitator = "https://x402.org/facilitator"
	defaultNetwork     = "base-sepolia"
	defaultPrice       = "0.01"
	defaultFullPrice   = "0.05"
	defaultBoardPrice  = "0.02"
)

// Config holds x402 paywall settings for the reputation oracle API.
type Config struct {
	Enabled         bool
	PayTo           string
	Network         string
	FacilitatorURL  string
	PriceSummary    string
	PriceFull       string
	PriceLeaderboard string
	OracleKey       string
	VerifyOnly      bool
}

// LoadConfig reads x402 settings from the environment.
// Paid routes are disabled when X402_PAY_TO is unset unless X402_BYPASS=1 (local dev).
func LoadConfig() Config {
	payTo := strings.TrimSpace(os.Getenv("X402_PAY_TO"))
	bypass := os.Getenv("X402_BYPASS") == "1"

	cfg := Config{
		Enabled:          payTo != "" || bypass,
		PayTo:            payTo,
		Network:          envOr("X402_NETWORK", defaultNetwork),
		FacilitatorURL:   envOr("X402_FACILITATOR_URL", defaultFacilitator),
		PriceSummary:     envOr("X402_PRICE", defaultPrice),
		PriceFull:        envOr("X402_PRICE_FULL", defaultFullPrice),
		PriceLeaderboard: envOr("X402_PRICE_LEADERBOARD", defaultBoardPrice),
		OracleKey:        firstNonEmpty(os.Getenv("ORACLE_PRIVATE_KEY"), os.Getenv("RELAYER_PRIVATE_KEY")),
		VerifyOnly:       os.Getenv("X402_VERIFY_ONLY") == "1",
	}

	if bypass && payTo == "" {
		cfg.PayTo = "0x0000000000000000000000000000000000000001"
	}

	return cfg
}

func (c Config) ChainConfig() x402.ChainConfig {
	switch strings.ToLower(c.Network) {
	case "base":
		return x402.BaseMainnet
	case "polygon":
		return x402.PolygonMainnet
	case "polygon-amoy":
		return x402.PolygonAmoy
	default:
		return x402.BaseSepolia
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
