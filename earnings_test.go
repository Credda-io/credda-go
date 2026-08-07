package credda

import (
	"context"
	"net/http"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestGetEarningsRequestShape(t *testing.T) {
	c, got := newTestServer(t, 200, `{
		"userId":"u_1","earningsVersion":"1.0","currency":null,
		"note":"values are platform-reported units",
		"window":{"from":"2025-08-01T00:00:00.000Z","to":"2026-07-15T12:00:00.000Z","months":12},
		"periods":[{"month":"2026-06","grossVerified":3200,"eventCount":4,"platformBreakdown":[{"platform":"upwork","gross":3200,"eventCount":4}]}],
		"attested":{"grossVerified":42000,"eventCount":40,"trailing12mTotal":42000,"platformCount":3,"platformBreakdown":[]},
		"stability":{"monthsWithEarnings":11,"medianMonthly":3200,"meanMonthly":3500,"coefficientOfVariation":0.42,"longestConsecutiveMonths":8},
		"unverifiedReported":{"gross":6000,"eventCount":5},
		"excluded":{"disputedEvents":1,"disputedValue":500,"valuelessEvents":0},
		"coverage":{"verifiedShare":0.87,"selfReportedShare":0.12},
		"disclosures":["not an income verification"]
	}`)

	out, err := c.GetEarnings(context.Background(), "u_1", &EarningsQuery{Months: intPtr(12)})
	if err != nil {
		t.Fatalf("GetEarnings: %v", err)
	}
	if got.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", got.Method)
	}
	if got.Path != "/api/v1/users/u_1/earnings" {
		t.Errorf("path = %s", got.Path)
	}
	if got.Query != "months=12" {
		t.Errorf("query = %s, want months=12", got.Query)
	}
	if got.Auth != "Bearer crd_test_key" {
		t.Errorf("auth = %s", got.Auth)
	}
	// Attested holds verified value only; unverified is reported separately.
	if out.Attested.GrossVerified != 42000 {
		t.Errorf("attested gross = %v", out.Attested.GrossVerified)
	}
	if out.UnverifiedReported.Gross != 6000 {
		t.Errorf("unverified gross = %v", out.UnverifiedReported.Gross)
	}
	if out.Excluded.DisputedEvents != 1 {
		t.Errorf("disputed events = %d", out.Excluded.DisputedEvents)
	}
	if len(out.Periods) != 1 || out.Periods[0].Month != "2026-06" {
		t.Errorf("periods = %+v", out.Periods)
	}
	if len(out.Disclosures) == 0 {
		t.Error("disclosures must always be present")
	}
}

func TestGetEarningsOmitsQueryWhenNoWindow(t *testing.T) {
	c, got := newTestServer(t, 200, `{"earningsVersion":"1.0"}`)
	if _, err := c.GetEarnings(context.Background(), "u_1", nil); err != nil {
		t.Fatalf("GetEarnings: %v", err)
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty", got.Query)
	}
}

func TestGetEarningsSummaryWindowAndNullables(t *testing.T) {
	c, got := newTestServer(t, 200, `{
		"earningsVersion":"1.0","trailing12mVerifiedTotal":42000,"medianMonthly":3200,
		"monthsWithEarnings":11,"volatility":null,"verifiedShare":null,"selfReportedShare":0.12,
		"platformCount":3,"longestConsecutiveMonths":8,"disclosures":["x"]
	}`)

	out, err := c.GetEarningsSummary(context.Background(), "u_1", &EarningsQuery{
		From: "2026-01-01T00:00:00Z",
		To:   "2026-06-30T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("GetEarningsSummary: %v", err)
	}
	if got.Path != "/api/v1/users/u_1/earnings/summary" {
		t.Errorf("path = %s", got.Path)
	}
	if got.Query != "from=2026-01-01T00%3A00%3A00Z&to=2026-06-30T00%3A00%3A00Z" {
		t.Errorf("query = %s", got.Query)
	}
	// Volatility is a pointer precisely so "no income to vary" stays distinct from 0.
	if out.Volatility != nil {
		t.Errorf("volatility = %v, want nil", *out.Volatility)
	}
	if out.VerifiedShare != nil {
		t.Errorf("verifiedShare = %v, want nil", *out.VerifiedShare)
	}
	if out.SelfReportedShare == nil || *out.SelfReportedShare != 0.12 {
		t.Errorf("selfReportedShare = %v", out.SelfReportedShare)
	}
}

func TestMintEarningsCredential(t *testing.T) {
	c, got := newTestServer(t, 201, `{
		"format":"jwt_vc_json","credentialVc":"ey.a.b","credentialType":"CreddaEarningsCredential",
		"issuer":"did:web:api.credda.io","earningsVersion":"1.0","claims":{"credentialKind":"verified-earnings"}
	}`)

	out, err := c.MintEarningsCredential(context.Background(), "u_1", &EarningsQuery{Months: intPtr(6)}, intPtr(600))
	if err != nil {
		t.Fatalf("MintEarningsCredential: %v", err)
	}
	if got.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.Method)
	}
	if got.Path != "/api/v1/users/u_1/earnings/credential" {
		t.Errorf("path = %s", got.Path)
	}
	if got.Body != `{"months":6,"ttlSeconds":600}` {
		t.Errorf("body = %s", got.Body)
	}
	if out.CredentialType != "CreddaEarningsCredential" {
		t.Errorf("credentialType = %s", out.CredentialType)
	}
	if out.CredentialVC == "" {
		t.Error("credentialVc must be present")
	}
}

func TestMintEarningsCredentialSurfacesTestModeRefusal(t *testing.T) {
	c, _ := newTestServer(t, 403, `{"error":"test mode","code":"TEST_MODE_NOT_ALLOWED"}`)
	if _, err := c.MintEarningsCredential(context.Background(), "u_1", nil, nil); err == nil {
		t.Fatal("expected an error for a test-mode refusal")
	}
}
