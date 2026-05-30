// signet-mcp exposes Signet reputation oracle tools over MCP with x402 micropayments.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/x402-go"
	x402mcp "github.com/mark3labs/x402-go/mcp/server"

	"server/internal/payapi"
	"server/internal/pulse"
	"server/internal/store"
)

func main() {
	ctx := context.Background()
	addr := envOr("MCP_ADDR", ":8787")
	signetAPI := os.Getenv("SIGNET_API_URL")

	st, err := store.Open(ctx)
	if err != nil {
		st = store.NewMemory()
	}
	if os.Getenv("SIGNET_DEV_SEED") == "1" {
		store.MaybeSeed(ctx, st)
	}

	cfg := payapi.LoadConfig()
	if cfg.PayTo == "" {
		log.Println("warning: X402_PAY_TO unset; payable tools will use bypass dev wallet")
		cfg = payapi.LoadConfig()
		os.Setenv("X402_BYPASS", "1")
		cfg = payapi.LoadConfig()
	}

	svc := payapi.NewService(st, cfg)
	_ = svc
	mcpCfg := x402mcp.DefaultConfig()
	mcpCfg.FacilitatorURL = cfg.FacilitatorURL
	mcpCfg.VerifyOnly = cfg.VerifyOnly

	s := x402mcp.NewX402Server("signet", "1.0.0", mcpCfg)

	summaryReq, _ := x402.NewUSDCPaymentRequirement(x402.USDCRequirementConfig{
		Chain: cfg.ChainConfig(), Amount: cfg.PriceSummary,
		RecipientAddress: cfg.PayTo, Description: "Signet reputation summary",
	})
	fullReq, _ := x402.NewUSDCPaymentRequirement(x402.USDCRequirementConfig{
		Chain: cfg.ChainConfig(), Amount: cfg.PriceFull,
		RecipientAddress: cfg.PayTo, Description: "Signet reputation full report",
	})
	boardReq, _ := x402.NewUSDCPaymentRequirement(x402.USDCRequirementConfig{
		Chain: cfg.ChainConfig(), Amount: cfg.PriceLeaderboard,
		RecipientAddress: cfg.PayTo, Description: "Signet leaderboard snapshot",
	})

	_ = s.AddPayableTool(mcp.NewTool("get_reputation",
		mcp.WithDescription("Fetch EIP-712 signed proof-of-code reputation summary for a wallet"),
		mcp.WithString("address", mcp.Required(), mcp.Description("Developer wallet (0x...)")),
	), makeReputationHandler(ctx, st, cfg, false), summaryReq)

	_ = s.AddPayableTool(mcp.NewTool("get_reputation_full",
		mcp.WithDescription("Full signed reputation report with per-attestation evidence"),
		mcp.WithString("address", mcp.Required(), mcp.Description("Developer wallet (0x...)")),
	), makeReputationHandler(ctx, st, cfg, true), fullReq)

	_ = s.AddPayableTool(mcp.NewTool("get_leaderboard",
		mcp.WithDescription("Signed top-N developer leaderboard snapshot"),
	), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		report, err := payapi.BuildLeaderboardReport(ctx, st, 20)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		sig, signer, err := payapi.SignLeaderboardReport(report, cfg.OracleKey)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, _ := json.Marshal(map[string]any{"report": report, "signature": sig, "signer": signer})
		return mcp.NewToolResultText(string(out)), nil
	}, boardReq)

	s.AddTool(mcp.NewTool("report_push",
		mcp.WithDescription("Submit a signed git push event JSON for AI review and attestation pipeline"),
		mcp.WithString("event_json", mcp.Required(), mcp.Description("JSON-encoded pulse event")),
	), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := req.RequireString("event_json")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if signetAPI != "" {
			return mcp.NewToolResultText(fmt.Sprintf(`{"status":"proxy","message":"POST event to %s/hook","payload":%s}`, signetAPI, raw)), nil
		}
		var e pulse.Event
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return mcp.NewToolResultError("invalid event_json: " + err.Error()), nil
		}
		if err := pulse.VerifyPushSignature(e); err != nil {
			return mcp.NewToolResultError("signature invalid: " + err.Error()), nil
		}
		if e.ID == "" {
			e = pulse.NewEvent()
		}
		if err := st.Insert(ctx, pulse.ToDomain(e)); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, _ := json.Marshal(map[string]string{"status": "queued", "event_id": e.ID})
		return mcp.NewToolResultText(string(out)), nil
	})

	s.AddTool(mcp.NewTool("signet_info",
		mcp.WithDescription("Free metadata about Signet oracle pricing and endpoints"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info := map[string]any{
			"name":        "Signet Reputation Oracle",
			"protocol":    "x402",
			"network":     cfg.Network,
			"facilitator": cfg.FacilitatorURL,
			"pricing": map[string]string{
				"get_reputation":      cfg.PriceSummary + " USDC",
				"get_reputation_full": cfg.PriceFull + " USDC",
				"get_leaderboard":     cfg.PriceLeaderboard + " USDC",
			},
		}
		b, _ := json.Marshal(info)
		return mcp.NewToolResultText(string(b)), nil
	})

	log.Printf("Signet MCP listening on %s (x402 network=%s)", addr, cfg.Network)
	if err := s.Start(addr); err != nil {
		log.Fatal(err)
	}
}

func makeReputationHandler(ctx context.Context, st store.Store, cfg payapi.Config, full bool) mcpserver.ToolHandlerFunc {
	return func(callCtx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		address, err := req.RequireString("address")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var report payapi.ReputationReport
		if full {
			report, err = payapi.BuildFullReport(ctx, st, address)
		} else {
			report, err = payapi.BuildSummaryReport(ctx, st, address)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		sig, signer, err := payapi.SignReputationReport(report, cfg.OracleKey)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, _ := json.Marshal(payapi.SignedResponse{Report: report, Signature: sig, Signer: signer, Schema: "SignetReputationReport/v1"})
		return mcp.NewToolResultText(string(out)), nil
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
