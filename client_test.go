package credda

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capture records what the fake server saw, so a test can assert on method,
// path, query, headers and body.
type capture struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Header http.Header
	Body   string
}

// newTestServer returns a client pointed at an httptest server that replies
// with `status` and `responseBody` for every request, plus the capture record.
func newTestServer(t *testing.T, status int, responseBody string) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.Method = r.Method
		got.Path = r.URL.Path
		got.Query = r.URL.RawQuery
		got.Auth = r.Header.Get("Authorization")
		got.Header = r.Header.Clone()
		got.Body = string(body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if responseBody != "" {
			_, _ = io.WriteString(w, responseBody)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithBaseURL(srv.URL), WithAPIKey("crd_test_key")), got
}

func TestClientRequestShapes(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
		wantNoAuth bool
	}{
		{
			name:     "ResolveToken",
			response: `{"token":"tok_1","finalScore":72,"scoreBand":"STRONG"}`,
			call: func(c *Client) error {
				_, err := c.ResolveToken(context.Background(), "tok_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_1",
			wantNoAuth: true,
		},
		{
			name:     "GetTrustExport",
			response: `{"format":"credda-trust-export/1"}`,
			call: func(c *Client) error {
				_, err := c.GetTrustExport(context.Background(), "tok_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_1/export",
			wantNoAuth: true,
		},
		{
			name:     "GetDeliveryReceipts",
			response: `{"token":"tok_1","subjectType":"agent","deliveryRecord":{"deliveries":12,"confirmedDeliveries":9,"selfAttestedDeliveries":3},"credentialVc":"eyJ.vc.jwt"}`,
			call: func(c *Client) error {
				_, err := c.GetDeliveryReceipts(context.Background(), "tok_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_1/delivery-receipts",
			wantNoAuth: true,
		},
		{
			name:     "RegisterAgent",
			response: `{"userId":"bot-1","subjectType":"agent"}`,
			call: func(c *Client) error {
				_, err := c.RegisterAgent(context.Background(), RegisterAgentInput{UserID: "bot-1", ModelFamily: "claude-opus-4-8"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/agents",
			wantBody:   `{"userId":"bot-1","modelFamily":"claude-opus-4-8"}`,
		},
		{
			name:     "RegisterAgent with a declared third-party operator",
			response: `{"userId":"bot-1","subjectType":"agent"}`,
			call: func(c *Client) error {
				operated := false
				_, err := c.RegisterAgent(context.Background(), RegisterAgentInput{
					UserID:                      "bot-1",
					OperatedByReportingPlatform: &operated,
					Operator:                    &AgentOperatorInput{Did: "did:web:acme.ai"},
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/agents",
			wantBody:   `{"userId":"bot-1","operatedByReportingPlatform":false,"operator":{"did":"did:web:acme.ai"}}`,
		},
		{
			name:     "GetAgent",
			response: `{"userId":"bot-1","subjectType":"agent","deliveryRecord":{"confirmedDeliveries":0}}`,
			call: func(c *Client) error {
				_, err := c.GetAgent(context.Background(), "bot-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/agents/bot-1",
		},
		{
			name:     "GetWebhookEvents",
			response: `{"eventTypes":["score.updated","score.band_changed","dispute.resolved"],"events":[{"type":"score.updated","description":"A score changed."}]}`,
			call: func(c *Client) error {
				_, err := c.GetWebhookEvents(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/webhooks/events",
			wantNoAuth: true,
		},
		{
			name:     "GetPlans",
			response: `{"pricing":"official","plans":[{"id":"STARTER","rateLimitPerMin":240,"priceUsdMonthly":49},{"id":"ENTERPRISE","rateLimitPerMin":1200,"priceUsdMonthly":1500}]}`,
			call: func(c *Client) error {
				_, err := c.GetPlans(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/plans",
			wantNoAuth: true,
		},
		{
			name:     "GetDIDDocument",
			response: `{"id":"did:web:api.credda.io"}`,
			call: func(c *Client) error {
				_, err := c.GetDIDDocument(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/.well-known/did.json",
			wantNoAuth: true,
		},
		{
			name:     "GetTrustRegistry",
			response: `{"version":"1","issuers":[]}`,
			call: func(c *Client) error {
				_, err := c.GetTrustRegistry(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/.well-known/credda-trust-registry.json",
			wantNoAuth: true,
		},
		{
			name:     "GetScore",
			response: `{"userId":"u_1","finalScore":72}`,
			call: func(c *Client) error {
				_, err := c.GetScore(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score",
		},
		{
			name:     "GetScoreExplain",
			response: `{"summary":"ok","factors":[]}`,
			call: func(c *Client) error {
				_, err := c.GetScoreExplain(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score/explain",
		},
		{
			name:     "GetScoreDelta",
			response: `{"userId":"u_1","available":false}`,
			call: func(c *Client) error {
				_, err := c.GetScoreDelta(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score/delta",
		},
		{
			name:     "GetScoreComponents",
			response: `{"userId":"u_1","available":true,"components":[]}`,
			call: func(c *Client) error {
				_, err := c.GetScoreComponents(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score/components",
		},
		{
			name:     "GetScoreHistory with filters",
			response: `{"data":[],"count":0}`,
			call: func(c *Client) error {
				_, err := c.GetScoreHistory(context.Background(), "u_1", &ScoreHistoryQuery{
					From: "2026-01-01", To: "2026-02-01", Limit: Int(25), Cursor: "cur_1",
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score/history",
			wantQuery:  "cursor=cur_1&from=2026-01-01&limit=25&to=2026-02-01",
		},
		{
			name:     "GetScoreHistory without filters",
			response: `{"data":[],"count":0}`,
			call: func(c *Client) error {
				_, err := c.GetScoreHistory(context.Background(), "u_1", nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/score/history",
			wantQuery:  "",
		},
		{
			name:     "GetTimeline",
			response: `{"data":[],"count":0,"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.GetTimeline(context.Background(), "u_1", &TimelineQuery{Limit: Int(10), Cursor: "c"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/timeline",
			wantQuery:  "cursor=c&limit=10",
		},
		{
			name:     "GetPlatforms",
			response: `{"platforms":[]}`,
			call: func(c *Client) error {
				_, err := c.GetPlatforms(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/platforms",
		},
		{
			name:     "GetRisk",
			response: `{"riskLevel":"LOW","riskScore":0,"signals":[],"advisory":true,"computedAt":"x"}`,
			call: func(c *Client) error {
				_, err := c.GetRisk(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u_1/risk",
		},
		{
			name:     "GetUsage with days",
			response: `{"rateLimitPerMin":60}`,
			call: func(c *Client) error {
				_, err := c.GetUsage(context.Background(), Int(30))
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/usage",
			wantQuery:  "days=30",
		},
		{
			name:     "GetActivity with filters",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.GetActivity(context.Background(), &ActivityQuery{Limit: Int(5), Action: "EVENT_CREATED", From: "2026-07-01"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/activity",
			wantQuery:  "action=EVENT_CREATED&from=2026-07-01&limit=5",
		},
		{
			name:     "ExportEvents with range",
			response: `{"data":[{"id":"e1","userId":"u-42","autoImported":false}],"nextCursor":"e1"}`,
			call: func(c *Client) error {
				_, err := c.ExportEvents(context.Background(), &EventExportQuery{From: "2026-07-01", To: "2026-07-22", Cursor: "e0"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/events/export",
			wantQuery:  "cursor=e0&from=2026-07-01&to=2026-07-22",
		},
		{
			name:     "GetScores batch",
			response: `{"scores":[],"count":0,"formulaVersion":"5.0"}`,
			call: func(c *Client) error {
				_, err := c.GetScores(context.Background(), []string{"u_1", "u_2"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/scores",
			wantBody:   `{"userIds":["u_1","u_2"]}`,
		},
		{
			name:     "ProjectScore",
			response: `{"userId":"u_1","delta":1.5}`,
			call: func(c *Client) error {
				_, err := c.ProjectScore(context.Background(), "u_1", []ProjectionEventInput{
					{EventType: EventContractFulfilled, StakeLevel: StakeHigh, IsVerified: Bool(true)},
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u_1/score/project",
			wantBody:   `{"events":[{"eventType":"CONTRACT_FULFILLED","stakeLevel":"HIGH","isVerified":true}]}`,
		},
		{
			name:     "ReportEvent",
			response: `{"event":{},"userId":"u_1"}`,
			call: func(c *Client) error {
				_, err := c.ReportEvent(context.Background(), ReportEventInput{
					UserID: "u_1", EventType: EventTransactionCompleted,
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/events",
			wantBody:   `{"userId":"u_1","eventType":"TRANSACTION_COMPLETED"}`,
		},
		{
			name:     "ReportEvents batch",
			response: `{"total":1,"created":1,"duplicate":0,"failed":0,"results":[]}`,
			call: func(c *Client) error {
				_, err := c.ReportEvents(context.Background(), []BatchEventInput{{
					ReportEventInput: ReportEventInput{UserID: "u_1", EventType: EventReviewVerified},
					IdempotencyKey:   "key-12345678",
				}})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/events/batch",
			wantBody:   `{"events":[{"userId":"u_1","eventType":"REVIEW_VERIFIED","idempotencyKey":"key-12345678"}]}`,
		},
		{
			name:     "MintShareToken",
			response: `{"token":"tok_1","verifyUrl":"https://credda.io/verify/tok_1"}`,
			call: func(c *Client) error {
				_, err := c.MintShareToken(context.Background(), "u_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u_1/share-token",
			wantBody:   `{}`,
		},
		{
			name:     "RevokeShareToken",
			response: "",
			call: func(c *Client) error {
				return c.RevokeShareToken(context.Background(), "u_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/users/u_1/share-token",
		},
		{
			name:     "ResolveDispute",
			response: `{"dispute":{}}`,
			call: func(c *Client) error {
				_, err := c.ResolveDispute(context.Background(), "dsp_1", DisputeForUser)
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/disputes/dsp_1/resolve",
			wantBody:   `{"outcome":"FOR_USER"}`,
		},
		{
			name:     "CreateWebhook",
			response: `{"webhook":{"id":"wh_1"},"secret":"whsec_x"}`,
			call: func(c *Client) error {
				_, err := c.CreateWebhook(context.Background(), CreateWebhookInput{
					URL: "https://example.com/hook", Events: []string{WebhookScoreUpdated},
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/webhooks",
			wantBody:   `{"url":"https://example.com/hook","events":["score.updated"]}`,
		},
		{
			name:     "ListWebhooks",
			response: `{"data":[]}`,
			call: func(c *Client) error {
				_, err := c.ListWebhooks(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/webhooks",
		},
		{
			name:     "UpdateWebhook",
			response: `{"webhook":{"id":"wh_1"}}`,
			call: func(c *Client) error {
				_, err := c.UpdateWebhook(context.Background(), "wh_1", UpdateWebhookInput{IsActive: Bool(false)})
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/webhooks/wh_1",
			wantBody:   `{"isActive":false}`,
		},
		{
			name:     "DeleteWebhook",
			response: "",
			call: func(c *Client) error {
				return c.DeleteWebhook(context.Background(), "wh_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/webhooks/wh_1",
		},
		{
			name:     "TestWebhook",
			response: `{"delivered":true,"statusCode":200,"error":null,"durationMs":12}`,
			call: func(c *Client) error {
				_, err := c.TestWebhook(context.Background(), "wh_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/webhooks/wh_1/test",
			wantBody:   `{}`,
		},
		{
			name:     "GetWebhookDeliveries",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.GetWebhookDeliveries(context.Background(), "wh_1", Int(5), "cur")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/webhooks/wh_1/deliveries",
			wantQuery:  "cursor=cur&limit=5",
		},
		{
			name:     "GetRecentWebhookEvents",
			response: `{"data":[],"nextCursor":null,"source":"deliveries"}`,
			call: func(c *Client) error {
				_, err := c.GetRecentWebhookEvents(context.Background(), Int(5), "", "score.updated", "dispute.resolved")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/webhooks/deliveries",
			wantQuery:  "eventType=score.updated%2Cdispute.resolved&limit=5",
		},
		{
			name:     "CreateMonitor",
			response: `{"monitor":{"id":"mon_1","userId":"u1","belowScore":40}}`,
			call: func(c *Client) error {
				_, err := c.CreateMonitor(context.Background(), CreateMonitorInput{UserID: "u1", BelowScore: Float(40)})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/monitors",
			wantBody:   `{"userId":"u1","belowScore":40}`,
		},
		{
			name:     "ListMonitors",
			response: `{"data":[{"id":"mon_1"}],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListMonitors(context.Background(), Int(10), "cur")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/monitors",
			wantQuery:  "cursor=cur&limit=10",
		},
		{
			name:     "GetMonitor",
			response: `{"monitor":{"id":"mon_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetMonitor(context.Background(), "mon_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/monitors/mon_1",
		},
		{
			name:     "UpdateMonitor",
			response: `{"monitor":{"id":"mon_1","isActive":false}}`,
			call: func(c *Client) error {
				_, err := c.UpdateMonitor(context.Background(), "mon_1", UpdateMonitorInput{IsActive: Bool(false)})
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/monitors/mon_1",
			wantBody:   `{"isActive":false}`,
		},
		{
			name:     "DeleteMonitor",
			response: "",
			call: func(c *Client) error {
				return c.DeleteMonitor(context.Background(), "mon_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/monitors/mon_1",
		},
		{
			name:     "CreateScreening",
			response: `{"screening":{"id":"scr_1","status":"COMPLETED","totalCount":2,"foundCount":1}}`,
			call: func(c *Client) error {
				_, err := c.CreateScreening(context.Background(), []string{"u_1", "u_2"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/screenings",
			wantBody:   `{"userIds":["u_1","u_2"]}`,
		},
		{
			name:     "ListScreenings",
			response: `{"data":[{"id":"scr_1","status":"RUNNING"}],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListScreenings(context.Background(), Int(10), "cur")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/screenings",
			wantQuery:  "cursor=cur&limit=10",
		},
		{
			name:     "GetScreening",
			response: `{"screening":{"id":"scr_1","status":"QUEUED"}}`,
			call: func(c *Client) error {
				_, err := c.GetScreening(context.Background(), "scr_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/screenings/scr_1",
		},
		{
			name:     "GetScreeningResults",
			response: `{"screening":{"id":"scr_1","status":"COMPLETED"},"results":[{"externalId":"u_1","found":true,"score":72,"band":"Good"},{"externalId":"ghost","found":false}],"count":2}`,
			call: func(c *Client) error {
				out, err := c.GetScreeningResults(context.Background(), "scr_1")
				if err != nil {
					return err
				}
				if out.Count != 2 || len(out.Results) != 2 {
					t.Errorf("results = %+v, want 2 items", out)
				}
				if out.Results[1].Found || out.Results[1].Score != nil {
					t.Errorf("not-found item = %+v, want Found=false with nil Score", out.Results[1])
				}
				return nil
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/screenings/scr_1/results",
		},
		{
			name:     "Ingest with an inline mapping",
			response: `{"total":1,"created":1,"duplicate":0,"failed":0,"results":[{"index":0,"userId":"w_1","status":"created","eventId":"evt_1"}]}`,
			call: func(c *Client) error {
				out, err := c.Ingest(
					context.Background(),
					[]any{map[string]any{"id": "w_1"}},
					IngestMapping{"userId": MappingPath("id")},
					"",
				)
				if err != nil {
					return err
				}
				if out.Created != 1 || out.Results[0].EventID != "evt_1" {
					t.Errorf("ingest = %+v, want 1 created", out)
				}
				return nil
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/ingest",
			wantBody:   `{"mapping":{"userId":"id"},"records":[{"id":"w_1"}]}`,
		},
		{
			name:     "Ingest with a stored mapping id surfaces per-record warnings",
			response: `{"total":1,"created":1,"duplicate":0,"failed":0,"results":[{"index":0,"userId":"w_1","status":"created","warnings":["isVerified downgraded to false: no counterparty evidence."]}]}`,
			call: func(c *Client) error {
				out, err := c.Ingest(context.Background(), []any{map[string]any{}}, nil, "map_1")
				if err != nil {
					return err
				}
				if len(out.Results[0].Warnings) != 1 {
					t.Errorf("warnings = %+v, want the isVerified downgrade", out.Results[0])
				}
				return nil
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/ingest",
			wantBody:   `{"mappingId":"map_1","records":[{}]}`,
		},
		{
			name:     "CreateMapping",
			response: `{"mapping":{"id":"map_1","name":"orders"}}`,
			call: func(c *Client) error {
				_, err := c.CreateMapping(context.Background(), "orders", "", IngestMapping{"userId": MappingPath("id")})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/ingest/mappings",
			wantBody:   `{"mapping":{"userId":"id"},"name":"orders"}`,
		},
		{
			name:     "ListMappings",
			response: `{"data":[{"id":"map_1","name":"orders"}],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListMappings(context.Background(), Int(5), "cur")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/ingest/mappings",
			wantQuery:  "cursor=cur&limit=5",
		},
		{
			name:     "GetMapping",
			response: `{"mapping":{"id":"map_1","name":"orders"}}`,
			call: func(c *Client) error {
				_, err := c.GetMapping(context.Background(), "map_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/ingest/mappings/map_1",
		},
		{
			name:     "DeleteMapping",
			response: "",
			call: func(c *Client) error {
				return c.DeleteMapping(context.Background(), "map_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/ingest/mappings/map_1",
		},
		{
			name:     "CreateImport",
			response: `{"import":{"id":"imp_1","status":"COMPLETED","totalRows":2,"createdCount":2,"skippedCount":0,"failedCount":0}}`,
			call: func(c *Client) error {
				out, err := c.CreateImport(context.Background(), "a\n1\n", nil, "map_1")
				if err != nil {
					return err
				}
				if out.Import.CreatedCount != 2 || out.Import.Status != "COMPLETED" {
					t.Errorf("import = %+v, want 2 created + COMPLETED", out.Import)
				}
				return nil
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/imports",
			wantBody:   `{"csv":"a\n1\n","mappingId":"map_1"}`,
		},
		{
			name:     "ListImports",
			response: `{"data":[{"id":"imp_1","status":"RUNNING"}],"nextCursor":"imp_0"}`,
			call: func(c *Client) error {
				_, err := c.ListImports(context.Background(), Int(2), "imp_9")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/imports",
			wantQuery:  "cursor=imp_9&limit=2",
		},
		{
			name:     "GetImport",
			response: `{"import":{"id":"imp_1","status":"QUEUED"}}`,
			call: func(c *Client) error {
				_, err := c.GetImport(context.Background(), "imp_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/imports/imp_1",
		},
		{
			name:     "GetImportErrors",
			response: `{"import":{"id":"imp_1","status":"COMPLETED","failedCount":1},"errors":[{"row":3,"error":"eventType: Required"}],"errorCount":1,"warnings":[{"row":2,"warning":"isVerified downgraded to false: no counterparty evidence.","userId":"w_2"}],"warningCount":1,"truncated":false}`,
			call: func(c *Client) error {
				out, err := c.GetImportErrors(context.Background(), "imp_1", Int(50), Int(0))
				if err != nil {
					return err
				}
				if out.Errors[0].Row != 3 || out.Truncated {
					t.Errorf("errors = %+v, want row 3 and truncated=false", out)
				}
				if len(out.Warnings) != 1 || out.Warnings[0].Row != 2 {
					t.Errorf("warnings = %+v, want the row-2 downgrade", out.Warnings)
				}
				return nil
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/imports/imp_1/errors",
			wantQuery:  "limit=50&offset=0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, got := newTestServer(t, http.StatusOK, tc.response)
			if err := tc.call(c); err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if got.Method != tc.wantMethod {
				t.Errorf("method = %q, want %q", got.Method, tc.wantMethod)
			}
			if got.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tc.wantPath)
			}
			if got.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q", got.Query, tc.wantQuery)
			}
			if tc.wantBody != "" && got.Body != tc.wantBody {
				t.Errorf("body = %s, want %s", got.Body, tc.wantBody)
			}
			if tc.wantNoAuth {
				if got.Auth != "" {
					t.Errorf("public endpoint sent an Authorization header: %q", got.Auth)
				}
			} else if got.Auth != "Bearer crd_test_key" {
				t.Errorf("auth = %q, want Bearer crd_test_key", got.Auth)
			}
		})
	}
}

func TestIdempotencyKeyHeader(t *testing.T) {
	c, got := newTestServer(t, http.StatusOK, `{"event":{},"userId":"u_1"}`)
	_, err := c.ReportEvent(context.Background(), ReportEventInput{
		UserID: "u_1", EventType: EventTransactionCompleted,
	}, "order-42")
	if err != nil {
		t.Fatalf("ReportEvent: %v", err)
	}
	if h := got.Header.Get("Idempotency-Key"); h != "order-42" {
		t.Fatalf("Idempotency-Key = %q, want order-42", h)
	}

	c2, got2 := newTestServer(t, http.StatusOK, `{"event":{},"userId":"u_1"}`)
	if _, err := c2.ReportEvent(context.Background(), ReportEventInput{
		UserID: "u_1", EventType: EventTransactionCompleted,
	}, ""); err != nil {
		t.Fatalf("ReportEvent: %v", err)
	}
	if _, ok := got2.Header["Idempotency-Key"]; ok {
		t.Fatal("Idempotency-Key sent when none was requested")
	}
}

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantMsg  string
		wantCode int
	}{
		{"error field", http.StatusNotFound, `{"error":"user not found"}`, "user not found", 404},
		{"message field", http.StatusBadRequest, `{"message":"bad request"}`, "bad request", 400},
		{"non-JSON body", http.StatusInternalServerError, `<html>oops</html>`, "request failed (500)", 500},
		{"empty body", http.StatusForbidden, ``, "request failed (403)", 403},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestServer(t, tc.status, tc.body)
			_, err := c.GetScore(context.Background(), "u_1")
			apiErr, ok := AsAPIError(err)
			if !ok {
				t.Fatalf("expected *APIError, got %v", err)
			}
			if apiErr.StatusCode != tc.wantCode {
				t.Errorf("status = %d, want %d", apiErr.StatusCode, tc.wantCode)
			}
			if apiErr.Message != tc.wantMsg {
				t.Errorf("message = %q, want %q", apiErr.Message, tc.wantMsg)
			}
			if !strings.Contains(apiErr.Error(), "/users/u_1/score") {
				t.Errorf("error string lost the path: %s", apiErr.Error())
			}
		})
	}
}

func TestDeleteSurfacesAPIError(t *testing.T) {
	c, _ := newTestServer(t, http.StatusNotFound, `{"error":"no such webhook"}`)
	err := c.DeleteWebhook(context.Background(), "wh_missing")
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.StatusCode != 404 || apiErr.Message != "no such webhook" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestMissingAPIKeyIsRejectedLocally(t *testing.T) {
	c := NewClient(WithBaseURL("http://127.0.0.1:1"))
	if _, err := c.GetScore(context.Background(), "u_1"); err == nil {
		t.Fatal("expected an error when no API key is configured")
	} else if !strings.Contains(err.Error(), "requires an API key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBaseURLTrailingSlashTrimmed(t *testing.T) {
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Path = r.URL.Path
		_, _ = io.WriteString(w, `{"token":"t"}`)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL + "///"))
	if _, err := c.ResolveToken(context.Background(), "t"); err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if got.Path != "/api/v1/verify/t" {
		t.Fatalf("path = %q, want /api/v1/verify/t", got.Path)
	}
}

func TestDecodesResponseBodies(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"u_1","finalScore":72.5,"scoreBand":"STRONG","confidence":0.82,
		"breakdown":{"cr":0.9,"otr":0.8,"dr":0.05,"vd":0.6,
			"platformTrustMultiplier":1.1,"consistencyFactor":0.95,"momentumFactor":1.01},
		"formulaVersion":"5.0","velocityFlag":false,"computedAt":"2026-07-20T00:00:00.000Z"}`)

	score, err := c.GetScore(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetScore: %v", err)
	}
	if score.FinalScore == nil || *score.FinalScore != 72.5 || score.ScoreBand == nil || *score.ScoreBand != "STRONG" {
		t.Fatalf("unexpected score: %+v", score)
	}
	if score.Breakdown == nil || score.Breakdown.CR != 0.9 || score.Breakdown.MomentumFactor != 1.01 {
		t.Fatalf("unexpected breakdown: %+v", score.Breakdown)
	}
	if score.FormulaVersion == nil || *score.FormulaVersion != "5.0" {
		t.Fatalf("unexpected formulaVersion: %v", score.FormulaVersion)
	}
}

func TestBatchScoresPartialSuccess(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"scores":[
		{"userId":"u_1","finalScore":72,"scoreBand":"STRONG","scoreFrozen":false},
		{"userId":"u_2","error":"not_found"}
	],"count":2,"formulaVersion":"5.0"}`)

	res, err := c.GetScores(context.Background(), []string{"u_1", "u_2"})
	if err != nil {
		t.Fatalf("GetScores: %v", err)
	}
	if len(res.Scores) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(res.Scores))
	}
	if res.Scores[0].Error != "" || res.Scores[0].FinalScore == nil || *res.Scores[0].FinalScore != 72 {
		t.Fatalf("unexpected first entry: %+v", res.Scores[0])
	}
	if res.Scores[1].Error != "not_found" || res.Scores[1].FinalScore != nil {
		t.Fatalf("unexpected second entry: %+v", res.Scores[1])
	}
}

func TestTimelineUnionDecoding(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"data":[
		{"type":"event","id":"e_1","occurredAt":"2026-07-20T00:00:00.000Z","eventType":"CONTRACT_FULFILLED",
		 "platformName":"Acme","isVerified":true,"stakeLevel":"HIGH","daysLate":null,"dispute":null},
		{"type":"score_change","id":"s_1","occurredAt":"2026-07-19T00:00:00.000Z","finalScore":72,
		 "scoreBand":"STRONG","scoreDelta":2,"direction":"up",
		 "topDriver":{"factor":"CR","before":0.8,"after":0.9,"delta":0.1,"improved":true}}
	],"count":2,"nextCursor":"2026-07-19T00:00:00.000Z"}`)

	tl, err := c.GetTimeline(context.Background(), "u_1", nil)
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	if len(tl.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(tl.Data))
	}
	ev, sc := tl.Data[0], tl.Data[1]
	if ev.Type != "event" || ev.EventType != "CONTRACT_FULFILLED" || ev.IsVerified == nil || !*ev.IsVerified {
		t.Fatalf("unexpected event item: %+v", ev)
	}
	if ev.DaysLate != nil || ev.Dispute != nil {
		t.Fatalf("expected null daysLate/dispute, got %+v", ev)
	}
	if sc.Type != "score_change" || sc.TopDriver == nil || sc.TopDriver.Factor != "CR" {
		t.Fatalf("unexpected score_change item: %+v", sc)
	}
	if tl.NextCursor == nil || *tl.NextCursor == "" {
		t.Fatal("expected a nextCursor")
	}
}

func TestPaginationCursorRoundTrip(t *testing.T) {
	var seen []string
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RawQuery)
		page++
		if page == 1 {
			_, _ = io.WriteString(w, `{"data":[{"finalScore":70}],"count":1,"nextCursor":"cur_2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"finalScore":71}],"count":1,"nextCursor":null}`)
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("k"))
	ctx := context.Background()

	first, err := c.GetScoreHistory(ctx, "u_1", &ScoreHistoryQuery{Limit: Int(1)})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if first.NextCursor == nil {
		t.Fatal("expected a nextCursor on page 1")
	}
	second, err := c.GetScoreHistory(ctx, "u_1", &ScoreHistoryQuery{Limit: Int(1), Cursor: *first.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if second.NextCursor != nil {
		t.Fatalf("expected exhaustion, got cursor %q", *second.NextCursor)
	}
	if len(seen) != 2 || seen[0] != "limit=1" || seen[1] != "cursor=cur_2&limit=1" {
		t.Fatalf("unexpected query sequence: %#v", seen)
	}
}

func TestRiskSignalKeepsUnknownFields(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"riskLevel":"MEDIUM","riskScore":31,
		"signals":[{"code":"VELOCITY","severity":"MEDIUM","detail":"burst","windowHours":1}],
		"advisory":true,"computedAt":"2026-07-20T00:00:00.000Z"}`)

	risk, err := c.GetRisk(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetRisk: %v", err)
	}
	if len(risk.Signals) != 1 || risk.Signals[0].Code != "VELOCITY" {
		t.Fatalf("unexpected signals: %+v", risk.Signals)
	}
	if risk.Signals[0].Extra["windowHours"] != float64(1) {
		t.Fatalf("extra field lost: %+v", risk.Signals[0].Extra)
	}
}

func TestUpdateWebhookOmitsUnsetFields(t *testing.T) {
	c, got := newTestServer(t, http.StatusOK, `{"webhook":{"id":"wh_1"}}`)
	if _, err := c.UpdateWebhook(context.Background(), "wh_1", UpdateWebhookInput{
		URL: String("https://example.com/new"),
	}); err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(got.Body), &body); err != nil {
		t.Fatalf("body was not JSON: %v", err)
	}
	if len(body) != 1 || body["url"] != "https://example.com/new" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestContextCancellation(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetScore(ctx, "u_1"); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
}
