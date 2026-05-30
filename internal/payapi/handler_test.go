package payapi

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mark3labs/x402-go"
	"github.com/mark3labs/x402-go/facilitator"
	x402http "github.com/mark3labs/x402-go/http"

	"server/internal/store"
)

func testOracleKey(t *testing.T) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(crypto.FromECDSA(key))
}

func testOracleKeyHex(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(crypto.FromECDSA(key)), key
}

func TestBuildSummaryReport(t *testing.T) {
	t.Setenv("SIGNET_DEV_SEED", "1")
	ctx := context.Background()
	st := store.NewMemory()
	store.MaybeSeed(ctx, st)

	report, err := BuildSummaryReport(ctx, st, "0xabc0000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalScore <= 0 {
		t.Fatalf("expected positive score, got %d", report.TotalScore)
	}
	if report.ReportHash == "" {
		t.Fatal("expected report hash")
	}
}

func TestSignVerifyReputationReport(t *testing.T) {
	privHex, _ := testOracleKeyHex(t)
	report := ReputationReport{
		Address:     "0xabc0000000000000000000000000000000000001",
		TotalScore:  420,
		CommitCount: 7,
		WeeklyDelta: 3,
		ReportHash:  "0x" + "ab"+repeat("cd", 31),
		IssuedAt:    "2026-05-31T00:00:00Z",
		Kind:        "summary",
	}
	sig, signer, err := SignReputationReport(report, privHex)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReputationReport(report, sig, signer); err != nil {
		t.Fatal(err)
	}
}

func TestMiddlewareReturns402WithoutPayment(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		PayTo:          "0x209693Bc6afc0C5328bA36FaF03C514EF312287C",
		Network:        "base-sepolia",
		FacilitatorURL: "http://mock.test",
		PriceSummary:   "0.01",
		OracleKey:      testOracleKey(t),
	}
	svc := NewService(store.NewMemory(), cfg)
	mw := svc.middleware("0.01", "test")

	mux := http.NewServeMux()
	mux.Handle("/v1/reputation/", mw(http.HandlerFunc(svc.routeReputation)))

	req := httptest.NewRequest(http.MethodGet, "/v1/reputation/0xabc0000000000000000000000000000000000001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}
}

func TestMiddlewarePaidRequestWithMockFacilitator(t *testing.T) {
	mockFac := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/verify":
			_ = json.NewEncoder(w).Encode(facilitator.VerifyResponse{
				IsValid: true,
				Payer:   "0x857b06519E91e3A54538791bDbb0E22373e36b66",
			})
		case "/settle":
			_ = json.NewEncoder(w).Encode(x402.SettlementResponse{
				Success:     true,
				Transaction: "0xdeadbeef",
				Network:     "base-sepolia",
				Payer:       "0x857b06519E91e3A54538791bDbb0E22373e36b66",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockFac.Close()

	privHex, _ := testOracleKeyHex(t)
	cfg := Config{
		Enabled:        true,
		PayTo:          "0x209693Bc6afc0C5328bA36FaF03C514EF312287C",
		Network:        "base-sepolia",
		FacilitatorURL: mockFac.URL,
		PriceSummary:   "0.01",
		OracleKey:      privHex,
		VerifyOnly:     true,
	}
	ctx := context.Background()
	t.Setenv("SIGNET_DEV_SEED", "1")
	st := store.NewMemory()
	store.MaybeSeed(ctx, st)
	svc := NewService(st, cfg)

	reqBody := x402.PaymentPayload{
		X402Version: 1,
		Scheme:      "exact",
		Network:     "base-sepolia",
		Payload: map[string]any{
			"amount":    "10000",
			"token":     "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
			"signature": "0xabcdef",
		},
	}
	raw, _ := json.Marshal(reqBody)
	paymentHeader := base64.StdEncoding.EncodeToString(raw)

	req, err := x402.NewUSDCPaymentRequirement(x402.USDCRequirementConfig{
		Chain:             cfg.ChainConfig(),
		Amount:            "0.01",
		RecipientAddress:  cfg.PayTo,
		Description:       "test",
		MaxTimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	mw := x402http.NewX402Middleware(&x402http.Config{
		FacilitatorURL:      mockFac.URL,
		PaymentRequirements: []x402.PaymentRequirement{req},
		VerifyOnly:          true,
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc.handleReputationSummary(w, r, "0xabc0000000000000000000000000000000000001")
	}))

	httpReq := httptest.NewRequest(http.MethodGet, "/v1/reputation/0xabc0000000000000000000000000000000000001", nil)
	httpReq.Header.Set("X-PAYMENT", paymentHeader)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp SignedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Signature == "" || resp.Signer == "" {
		t.Fatal("expected signed response")
	}
	if err := VerifyReputationReport(resp.Report, resp.Signature, resp.Signer); err != nil {
		t.Fatal(err)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
