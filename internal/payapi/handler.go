package payapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/x402-go"
	"github.com/mark3labs/x402-go/facilitator"
	x402http "github.com/mark3labs/x402-go/http"

	"server/internal/store"
)

// Service holds paid reputation oracle handlers.
type Service struct {
	st     store.Store
	cfg    Config
	meter  *Meter
	signer string
}

// NewService creates a payapi service.
func NewService(st store.Store, cfg Config) *Service {
	s := &Service{
		st:    st,
		cfg:   cfg,
		meter: NewMeter(10000),
	}
	if cfg.OracleKey != "" {
		_, addr, err := SignReputationReport(ReputationReport{
			Address:    "0x0000000000000000000000000000000000000001",
			ReportHash: "0x" + strings.Repeat("00", 32),
			IssuedAt:   "1970-01-01T00:00:00Z",
			Kind:       "probe",
		}, cfg.OracleKey)
		if err == nil {
			s.signer = addr
		}
	}
	return s
}

// Meter returns the query receipt store.
func (s *Service) Meter() *Meter {
	return s.meter
}

// HandleRoutes registers /v1/* routes on mux, optionally wrapped with x402 middleware.
func (s *Service) HandleRoutes(mux *http.ServeMux) {
	if !s.cfg.Enabled {
		mux.HandleFunc("/v1/", s.disabledHandler)
		mux.HandleFunc("/v1", s.docsHandler)
		return
	}

	bypass := os.Getenv("X402_BYPASS") == "1"
	if bypass {
		mux.HandleFunc("/v1/reputation/", s.routeReputation)
		mux.HandleFunc("/v1/leaderboard", s.handleLeaderboard)
		mux.HandleFunc("/v1", s.docsHandler)
		mux.HandleFunc("/v1/docs", s.docsHandler)
		mux.HandleFunc("/v1/meter", s.handleMeter)
		return
	}

	summaryMW := s.middleware(s.cfg.PriceSummary, "Signet reputation summary report")
	fullMW := s.middleware(s.cfg.PriceFull, "Signet reputation full report with attestation evidence")
	boardMW := s.middleware(s.cfg.PriceLeaderboard, "Signet leaderboard snapshot")

	mux.Handle("/v1/reputation/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/full") {
			fullMW(http.HandlerFunc(s.routeReputation)).ServeHTTP(w, r)
			return
		}
		summaryMW(http.HandlerFunc(s.routeReputation)).ServeHTTP(w, r)
	}))
	mux.Handle("/v1/leaderboard", boardMW(http.HandlerFunc(s.handleLeaderboard)))
	mux.HandleFunc("/v1", s.docsHandler)
	mux.HandleFunc("/v1/docs", s.docsHandler)
	mux.HandleFunc("/v1/meter", s.handleMeter)
}

func (s *Service) middleware(amount, description string) func(http.Handler) http.Handler {
	req, err := x402.NewUSDCPaymentRequirement(x402.USDCRequirementConfig{
		Chain:             s.cfg.ChainConfig(),
		Amount:            amount,
		RecipientAddress:  s.cfg.PayTo,
		Description:       description,
		MaxTimeoutSeconds: 60,
	})
	if err != nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return x402http.NewX402Middleware(&x402http.Config{
		FacilitatorURL:      s.cfg.FacilitatorURL,
		PaymentRequirements: []x402.PaymentRequirement{req},
		VerifyOnly:          s.cfg.VerifyOnly,
	})
}

func (s *Service) routeReputation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/reputation/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	addr := parts[0]
	if !strings.HasPrefix(strings.ToLower(addr), "0x") {
		http.Error(w, "invalid address", http.StatusBadRequest)
		return
	}
	full := len(parts) >= 2 && parts[1] == "full"
	if full {
		s.handleReputationFull(w, r, addr)
		return
	}
	if len(parts) > 1 {
		http.NotFound(w, r)
		return
	}
	s.handleReputationSummary(w, r, addr)
}

func (s *Service) handleReputationSummary(w http.ResponseWriter, r *http.Request, addr string) {
	report, err := BuildSummaryReport(r.Context(), s.st, addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeSignedReputation(w, r, report)
}

func (s *Service) handleReputationFull(w http.ResponseWriter, r *http.Request, addr string) {
	report, err := BuildFullReport(r.Context(), s.st, addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeSignedReputation(w, r, report)
}

func (s *Service) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report, err := BuildLeaderboardReport(r.Context(), s.st, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sig, signer, err := SignLeaderboardReport(report, s.cfg.OracleKey)
	if err != nil {
		http.Error(w, "oracle signing unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	s.recordPayment(r, "/v1/leaderboard", s.cfg.PriceLeaderboard)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"report":    report,
		"signature": sig,
		"signer":    signer,
		"schema":    reportSchema,
	})
}

func (s *Service) handleMeter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"stats":   s.meter.Stats(),
		"recent":  s.meter.Recent(20),
	})
}

func (s *Service) writeSignedReputation(w http.ResponseWriter, r *http.Request, report ReputationReport) {
	sig, signer, err := SignReputationReport(report, s.cfg.OracleKey)
	if err != nil {
		http.Error(w, "oracle signing unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	route := "/v1/reputation/" + report.Address
	if report.Kind == "full" {
		route += "/full"
	}
	price := s.cfg.PriceSummary
	if report.Kind == "full" {
		price = s.cfg.PriceFull
	}
	s.recordPayment(r, route, price)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SignedResponse{
		Report:    report,
		Signature: sig,
		Signer:    signer,
		Schema:    reportSchema,
	})
}

func (s *Service) recordPayment(r *http.Request, route, amount string) {
	if v := r.Context().Value(x402http.PaymentContextKey); v != nil {
		if verify, ok := v.(*facilitator.VerifyResponse); ok && verify != nil {
			s.meter.Record(Receipt{
				Payer:  verify.Payer,
				Route:  route,
				Amount: amount,
			})
		}
	}
}

func (s *Service) disabledHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "paid_api_disabled",
		"message": "Set X402_PAY_TO to enable the reputation oracle API",
		"docs":    "/v1/docs",
	})
}

func (s *Service) docsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":        "Signet Reputation Oracle",
		"version":     "v1",
		"protocol":    "x402",
		"enabled":     s.cfg.Enabled && s.cfg.PayTo != "" && os.Getenv("X402_BYPASS") != "1",
		"network":     s.cfg.Network,
		"facilitator": s.cfg.FacilitatorURL,
		"oracle":      s.signer,
		"pricing": map[string]string{
			"GET /v1/reputation/{addr}":      s.cfg.PriceSummary + " USDC",
			"GET /v1/reputation/{addr}/full": s.cfg.PriceFull + " USDC",
			"GET /v1/leaderboard":            s.cfg.PriceLeaderboard + " USDC",
		},
		"flow": []string{
			"1. GET endpoint without X-PAYMENT header",
			"2. Server returns 402 with payment requirements",
			"3. Sign EIP-3009 USDC authorization and retry with X-PAYMENT header",
			"4. Facilitator verifies and settles; server returns signed report",
		},
	})
}
