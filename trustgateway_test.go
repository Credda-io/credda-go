package credda

import (
	"context"
	"net/http"
	"testing"
)

// TestTrustGatewayRequestShapes asserts method + path + query + body + auth for
// the Trust Gateway surface against the mounted routes (routes/trust.ts, where
// the watch paths come from a nested router.use('/watches', watchRouter)).
func TestTrustGatewayRequestShapes(t *testing.T) {
	limit := 25
	version := 3
	name := "Renamed"
	inactive := false
	explain := true

	tests := []struct {
		name       string
		response   string
		call       func(c *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name:     "CreateTrustPolicy",
			response: `{"id":"tp_1","name":"Vendor bar","version":1,"isActive":true}`,
			call: func(c *Client) error {
				_, err := c.CreateTrustPolicy(context.Background(), CreateTrustPolicyInput{
					Name:  "Vendor bar",
					Rules: map[string]any{"field": "score", "operator": "gte", "value": 80},
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/trust/policies",
			wantBody:   `{"name":"Vendor bar","rules":{"field":"score","operator":"gte","value":80}}`,
		},
		{
			name:     "ListTrustPolicies",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListTrustPolicies(context.Background(), &limit, "cur_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/trust/policies",
			wantQuery:  "cursor=cur_1&limit=25",
		},
		{
			name:     "GetTrustPolicy at a past version",
			response: `{"id":"tp_1","name":"Vendor bar","version":3}`,
			call: func(c *Client) error {
				_, err := c.GetTrustPolicy(context.Background(), "tp_1", &version)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/trust/policies/tp_1",
			wantQuery:  "version=3",
		},
		{
			name:     "UpdateTrustPolicy",
			response: `{"id":"tp_1","name":"Renamed","version":1,"isActive":false}`,
			call: func(c *Client) error {
				_, err := c.UpdateTrustPolicy(context.Background(), "tp_1", UpdateTrustPolicyInput{
					Name: &name, IsActive: &inactive,
				})
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/trust/policies/tp_1",
			wantBody:   `{"name":"Renamed","isActive":false}`,
		},
		{
			name:     "EvaluateTrustPolicy",
			response: `{"result":"satisfied","subject":{"reference":"crd_share_abcd"},"evaluatedAt":"2026-08-14T00:00:00Z","livemode":true,"note":"evidence, not a decision"}`,
			call: func(c *Client) error {
				_, err := c.EvaluateTrustPolicy(context.Background(), EvaluateTrustPolicyInput{
					Subject: "crd_share_abcd", Policy: "tp_1", Explain: &explain,
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/trust/evaluate",
			wantBody:   `{"subject":"crd_share_abcd","policy":"tp_1","explain":true}`,
		},
		{
			name:     "CreateTrustWatch",
			response: `{"id":"tw_1","policy":"tp_1","subject":{"reference":"crd_share_…abcd"},"authorizationBasis":"share_token","lastResult":null,"isActive":true}`,
			call: func(c *Client) error {
				_, err := c.CreateTrustWatch(context.Background(), "crd_share_abcd", "tp_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/trust/watches",
			wantBody:   `{"policy":"tp_1","subject":"crd_share_abcd"}`,
		},
		{
			name:     "ListTrustWatches",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListTrustWatches(context.Background(), &limit, "")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/trust/watches",
			wantQuery:  "limit=25",
		},
		{
			name:     "DeleteTrustWatch",
			response: `{}`,
			call: func(c *Client) error {
				return c.DeleteTrustWatch(context.Background(), "tw_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/trust/watches/tw_1",
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
			// Every Trust Gateway endpoint is keyed; none is public.
			if got.Auth != "Bearer crd_test_key" {
				t.Errorf("auth = %q, want Bearer crd_test_key", got.Auth)
			}
		})
	}
}

// TestEvaluateDecodesInsufficientEvidence pins the third value: an unmeasured
// subject has not FAILED the policy, and the two must stay distinguishable.
func TestEvaluateDecodesInsufficientEvidence(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"result":"insufficient_evidence",
		"policy":{"id":"tp_1","version":2},
		"subject":{"reference":"user-42"},
		"evaluatedAt":"2026-08-14T00:00:00Z",
		"livemode":false,
		"checks":[{"field":"score","operator":"gte","value":80,"result":"insufficient_evidence"}],
		"note":"evidence, not a decision"
	}`)
	out, err := c.EvaluateTrustPolicy(context.Background(), EvaluateTrustPolicyInput{Subject: "user-42", Policy: "tp_1"})
	if err != nil {
		t.Fatalf("EvaluateTrustPolicy: %v", err)
	}
	if out.Result != TrustResultInsufficientEvidence {
		t.Fatalf("result = %q, want %q", out.Result, TrustResultInsufficientEvidence)
	}
	if out.Result == TrustResultNotSatisfied {
		t.Fatal("insufficient_evidence collapsed into not_satisfied")
	}
	if out.Policy == nil || out.Policy.Version != 2 {
		t.Fatalf("policy ref = %+v, want version 2", out.Policy)
	}
	if len(out.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(out.Checks))
	}
}

// TestWatchDecodesNullLastResultAsNil pins "never delivered" staying distinct
// from a delivered result; decoded into a string, null would become "".
func TestWatchDecodesNullLastResultAsNil(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"data":[
		{"id":"tw_1","policy":"tp_1","subject":{"reference":"crd_share_…abcd"},"authorizationBasis":"share_token","lastResult":null,"lastResultAt":null,"isActive":true,"revokedAt":null,"livemode":true,"createdAt":"2026-08-14T00:00:00Z"},
		{"id":"tw_2","policy":"tp_1","subject":{"reference":"user-42"},"authorizationBasis":"platform_relationship","lastResult":"satisfied","lastResultAt":"2026-08-14T01:00:00Z","isActive":true,"revokedAt":null,"livemode":true,"createdAt":"2026-08-14T00:00:00Z"}
	],"nextCursor":null}`)
	out, err := c.ListTrustWatches(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("ListTrustWatches: %v", err)
	}
	if len(out.Data) != 2 {
		t.Fatalf("data = %d, want 2", len(out.Data))
	}
	if out.Data[0].LastResult != nil {
		t.Fatalf("lastResult = %v, want nil", *out.Data[0].LastResult)
	}
	if out.Data[1].LastResult == nil || *out.Data[1].LastResult != TrustResultSatisfied {
		t.Fatalf("lastResult = %v, want satisfied", out.Data[1].LastResult)
	}
	if out.NextCursor != nil {
		t.Fatalf("nextCursor = %v, want nil", out.NextCursor)
	}
}
