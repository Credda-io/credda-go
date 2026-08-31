package credda

import (
	"context"
	"net/http"
	"testing"
)

// TestRequestShapes asserts the method, path, query string and body of every
// method in this package against the routes the engine actually mounts.
//
// # WHAT THIS FILE IS AND IS NOT
//
// It used to be a cross-SDK parity test: it claimed to hold this client to the
// same request shapes as the TypeScript SDK. It did not do that. It was a
// hardcoded table of expected shapes, checked against nothing but itself, and a
// table like that agrees with whatever it was written next to. Cross-SDK parity
// was in the name and never in the assertions.
//
// It is now checked against the one thing that can falsify it: the engine's own
// route modules. Every want below is copied from a mount in
// apps/api/src/app.ts and the Hono router in the named file, and a wrong path
// or a mis-spelled query parameter fails here rather than at a customer's
// deployment.
//
// WHAT IT DOES NOT COVER, AND WHAT NOW DOES.
//
// This table lists what this client CALLS, so a route the ENGINE gains fails
// nothing here. POST /api/investigations/{id}/cancel shipped in core on
// 2026-08-29 and left credda-js green at 102 tests with no method for it; this
// table would have been just as quiet. That gap is now closed in
// surface_test.go, which loads route-surface.json -- generated in core from
// apps/api/src/openapi.ts and copied here, not transcribed -- and fails when
// the engine serves a route this package has neither a method nor a stated
// reason for. The fixture carries method, path and status codes and nothing
// else, which is why this file still exists: query parameter names, body
// encoding and the omission of an absent optional field are not in it, and a
// wrong one of those is a customer's 400.
//
// So the two are complementary and neither is redundant. The fixture answers
// "does the client know about every route"; this table answers "does it call
// the routes it knows about correctly".
//
// Query strings are url.Values.Encode() output, which sorts alphabetically.
func TestRequestShapes(t *testing.T) {
	tests := []struct {
		name string
		// route names the engine file the expectation was read from.
		route      string
		response   string
		call       func(*Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		// ── investigations (routes/investigations.ts) ──────────────────────
		{
			name:     "ListInvestigations, no filters",
			route:    "routes/investigations.ts app.get('/')",
			response: `{"investigations":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListInvestigations(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations",
		},
		{
			name:     "CancelInvestigation, no reason",
			route:    "routes/investigations.ts app.post('/:id/cancel')",
			response: `{"investigationId":"inv_1","state":"CANCELLED","status":"CANCELLED"}`,
			call: func(c *Client) error {
				_, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/investigations/inv_1/cancel",
			// `{}`, not `{"reason":""}`: cancelBody is strict and requires 1 to
			// 500 characters when reason is present, so an empty string would
			// be a 400 for a caller who simply said nothing.
			wantBody: `{}`,
		},
		{
			name:     "CancelInvestigation with a reason",
			route:    "routes/investigations.ts cancelBody",
			response: `{"investigationId":"inv_1","state":"CANCELLED","status":"CANCELLED"}`,
			call: func(c *Client) error {
				_, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{Reason: "wrong repository"})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/investigations/inv_1/cancel",
			wantBody:   `{"reason":"wrong repository"}`,
		},
		{
			name:     "ListInvestigations with state and paging",
			route:    "routes/investigations.ts listQuery",
			response: `{"investigations":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListInvestigations(context.Background(), &InvestigationQuery{
					State: "REPRODUCED_AND_DIAGNOSED",
					Page:  Page{Limit: Int(25), Offset: Int(50)},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations",
			wantQuery:  "limit=25&offset=50&state=REPRODUCED_AND_DIAGNOSED",
		},
		{
			name:     "ListInvestigations with repository, signal and outcome",
			route:    "routes/investigations.ts listQuery",
			response: `{"investigations":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListInvestigations(context.Background(), &InvestigationQuery{
					Repository: "repo_1",
					Signal:     "sig_1",
					Outcome:    "RESOLVED",
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations",
			wantQuery:  "outcome=RESOLVED&repository=repo_1&signal=sig_1",
		},
		{
			name:     "CreateInvestigation",
			route:    "routes/investigations.ts app.post('/') createBody",
			response: `{"investigation":{"id":"inv_1","state":"CREATED"},"hypotheses":[],"patches":[],"verifications":[],"evidenceCount":0,"latestSequence":0}`,
			call: func(c *Client) error {
				_, err := c.CreateInvestigation(context.Background(), CreateInvestigationInput{
					RepositoryID: "repo_1",
					IssueTitle:   "Checkout returns 500 on submit",
					IssueBody:    "Since Tuesday, POST /checkout 500s for cart totals over 1000.",
					IssueRef:     String("https://tracker.example/ISSUE-41"),
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/investigations",
			wantBody: `{"repositoryId":"repo_1","issueTitle":"Checkout returns 500 on submit",` +
				`"issueBody":"Since Tuesday, POST /checkout 500s for cart totals over 1000.",` +
				`"issueRef":"https://tracker.example/ISSUE-41"}`,
		},
		{
			// issueRef is optional in createBody, so it must be OMITTED rather
			// than sent as null: zod's .optional() rejects an explicit null.
			name:     "CreateInvestigation omits an absent issueRef",
			route:    "routes/investigations.ts createBody issueRef.optional()",
			response: `{"investigation":{"id":"inv_2"},"hypotheses":[],"patches":[],"verifications":[],"evidenceCount":0,"latestSequence":0}`,
			call: func(c *Client) error {
				_, err := c.CreateInvestigation(context.Background(), CreateInvestigationInput{
					RepositoryID: "repo_1", IssueTitle: "t", IssueBody: "",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/investigations",
			wantBody:   `{"repositoryId":"repo_1","issueTitle":"t","issueBody":""}`,
		},
		{
			name:     "GetInvestigation",
			route:    "routes/investigations.ts app.get('/:id')",
			response: `{"investigation":{"id":"inv_1"},"hypotheses":[],"patches":[],"verifications":[],"evidenceCount":0,"latestSequence":0}`,
			call: func(c *Client) error {
				_, err := c.GetInvestigation(context.Background(), "inv_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations/inv_1",
		},
		{
			name:     "InvestigationEvents with cursor and debug",
			route:    "routes/investigations.ts eventsQuery",
			response: `{"events":[],"latestSequence":0,"nextSince":0,"hasMore":false}`,
			call: func(c *Client) error {
				_, err := c.InvestigationEvents(context.Background(), "inv_1", &EventQuery{
					Since: Int(120), Limit: Int(500), IncludeDebug: Bool(true),
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations/inv_1/events",
			wantQuery:  "includeDebug=true&limit=500&since=120",
		},
		{
			name:     "InvestigationEvidence with type filter",
			route:    "routes/investigations.ts evidenceQuery",
			response: `{"evidence":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.InvestigationEvidence(context.Background(), "inv_1", &EvidenceQuery{
					Type: "VULNERABILITY", Page: Page{Limit: Int(10)},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/investigations/inv_1/evidence",
			wantQuery:  "limit=10&type=VULNERABILITY",
		},

		// ── repositories (routes/repositories.ts) ──────────────────────────
		{
			name:     "ListRepositories",
			route:    "routes/repositories.ts app.get('/')",
			response: `{"repositories":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListRepositories(context.Background(), &Page{Limit: Int(100)})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/repositories",
			wantQuery:  "limit=100",
		},
		{
			name:     "GetRepository",
			route:    "routes/repositories.ts app.get('/:id')",
			response: `{"repository":{"id":"repo_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetRepository(context.Background(), "repo_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/repositories/repo_1",
		},
		{
			name:     "RepositoryLearnings with kind",
			route:    "routes/repositories.ts learningsQuery",
			response: `{"learnings":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.RepositoryLearnings(context.Background(), "repo_1", &LearningQuery{
					Kind: "FRAGILE_SITE", Page: Page{Offset: Int(20)},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/repositories/repo_1/learnings",
			wantQuery:  "kind=FRAGILE_SITE&offset=20",
		},

		// ── resolutions (routes/resolutions.ts) ────────────────────────────
		{
			name:     "ListResolutions with every filter",
			route:    "routes/resolutions.ts listQuery",
			response: `{"resolutions":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListResolutions(context.Background(), &ResolutionQuery{
					Investigation: "inv_1",
					Signal:        "sig_1",
					Confidence:    "NOT_ESTABLISHED",
					Page:          Page{Limit: Int(50), Offset: Int(0)},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/resolutions",
			wantQuery:  "confidence=NOT_ESTABLISHED&investigation=inv_1&limit=50&offset=0&signal=sig_1",
		},
		{
			// `latest` is registered ahead of `/:id` in the engine's router, so
			// it is a literal segment and never an id.
			name:     "LatestResolution",
			route:    "routes/resolutions.ts app.get('/latest')",
			response: `{"resolution":null}`,
			call: func(c *Client) error {
				_, err := c.LatestResolution(context.Background(), "inv_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/resolutions/latest",
			wantQuery:  "investigation=inv_1",
		},
		{
			name:     "GetResolution",
			route:    "routes/resolutions.ts app.get('/:id')",
			response: `{"resolution":{"id":"res_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetResolution(context.Background(), "res_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/resolutions/res_1",
		},

		// ── validations (routes/validations.ts) ────────────────────────────
		{
			name:     "ListValidations with every filter",
			route:    "routes/validations.ts listQuery",
			response: `{"validations":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ListValidations(context.Background(), &ValidationQuery{
					Repository: "repo_1", State: "COMPLETED", Outcome: "BLOCKED",
					Page: Page{Limit: Int(10)},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations",
			wantQuery:  "limit=10&outcome=BLOCKED&repository=repo_1&state=COMPLETED",
		},
		{
			name:     "GetValidation",
			route:    "routes/validations.ts app.get('/:id')",
			response: `{"validation":{"id":"val_1"},"environment":{"status":"READY"},"changeImpact":{},"checkCount":0,"findingCount":0,"evidenceCount":0,"latestSequence":0}`,
			call: func(c *Client) error {
				_, err := c.GetValidation(context.Background(), "val_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1",
		},
		{
			name:     "ValidationChecks",
			route:    "routes/validations.ts app.get('/:id/checks')",
			response: `{"checks":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ValidationChecks(context.Background(), "val_1", &Page{Limit: Int(100), Offset: Int(0)})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/checks",
			wantQuery:  "limit=100&offset=0",
		},
		{
			name:     "ValidationFindings",
			route:    "routes/validations.ts app.get('/:id/findings')",
			response: `{"findings":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ValidationFindings(context.Background(), "val_1", nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/findings",
		},
		{
			name:     "ValidationEvidence",
			route:    "routes/validations.ts app.get('/:id/evidence')",
			response: `{"evidence":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ValidationEvidence(context.Background(), "val_1", nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/evidence",
		},
		{
			name:     "ValidationFindings with severity and status",
			route:    "routes/validations.ts findingsQuery",
			response: `{"findings":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ValidationFindings(context.Background(), "val_1", &FindingQuery{
					Severity: "HIGH", Status: "OPEN",
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/findings",
			wantQuery:  "severity=HIGH&status=OPEN",
		},
		{
			name:     "ValidationEvidence with type",
			route:    "routes/validations.ts evidenceQuery",
			response: `{"evidence":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.ValidationEvidence(context.Background(), "val_1", &EvidenceQuery{
					Type: "TEST_RESULT",
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/evidence",
			wantQuery:  "type=TEST_RESULT",
		},
		{
			name:     "ValidationEvents",
			route:    "routes/validations.ts eventsQuery",
			response: `{"events":[],"latestSequence":0,"nextSince":0,"hasMore":false}`,
			call: func(c *Client) error {
				_, err := c.ValidationEvents(context.Background(), "val_1", &EventQuery{Since: Int(9)})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/validations/val_1/events",
			wantQuery:  "since=9",
		},

		// ── organization (routes/organization.ts) ──────────────────────────
		{
			name:     "GetOrganization",
			route:    "routes/organization.ts app.get('/')",
			response: `{"organization":{"id":"org_1","name":"Acme","slug":"acme","createdAt":"2026-01-01T00:00:00.000Z"},"memberCount":0,"apiKeyCount":1,"revokedApiKeyCount":0,"repositoryCount":0,"investigationCount":0,"validationCount":0}`,
			call: func(c *Client) error {
				_, err := c.GetOrganization(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/organization",
		},
		{
			name:     "OrganizationMembers",
			route:    "routes/organization.ts app.get('/members')",
			response: `{"members":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.OrganizationMembers(context.Background(), &Page{Limit: Int(50)})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/organization/members",
			wantQuery:  "limit=50",
		},
		{
			name:     "OrganizationKeys",
			route:    "routes/organization.ts app.get('/keys')",
			response: `{"keys":[],"total":0}`,
			call: func(c *Client) error {
				_, err := c.OrganizationKeys(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/organization/keys",
		},

		// ── health and metrics (routes/health.ts, routes/metrics.ts, app.ts) ─
		{
			name:     "GetHealth",
			route:    "routes/health.ts app.get('/')",
			response: `{"status":"ok","schemaVersion":42,"expectedSchemaVersion":42,"checks":[]}`,
			call: func(c *Client) error {
				_, err := c.GetHealth(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/health",
		},
		{
			name:     "Metrics",
			route:    "routes/metrics.ts app.get('/')",
			response: `# nothing`,
			call: func(c *Client) error {
				_, err := c.Metrics(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/metrics",
		},
		{
			name:     "Livez is outside /api",
			route:    "app.ts app.get('/livez')",
			response: ``,
			call: func(c *Client) error {
				return c.Livez(context.Background())
			},
			wantMethod: http.MethodGet,
			wantPath:   "/livez",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, got := newTestServer(t, 200, tc.response)
			if err := tc.call(c); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.Method != tc.wantMethod {
				t.Errorf("method = %s, want %s (%s)", got.Method, tc.wantMethod, tc.route)
			}
			if got.Path != tc.wantPath {
				t.Errorf("path = %s, want %s (%s)", got.Path, tc.wantPath, tc.route)
			}
			if got.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q (%s)", got.Query, tc.wantQuery, tc.route)
			}
			if got.Body != tc.wantBody {
				t.Errorf("body  = %s\nwant  = %s\n(%s)", got.Body, tc.wantBody, tc.route)
			}
		})
	}
}

// A nil Page must send NOTHING rather than limit=0. The engine's zod schema is
// `positive()`, so limit=0 is a 400 — a client that helpfully filled in a
// default would turn every unfiltered list call into a validation failure.
func TestNilPagingSendsNoQueryString(t *testing.T) {
	c, got := newTestServer(t, 200, `{"repositories":[],"total":0}`)
	if _, err := c.ListRepositories(context.Background(), nil); err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if got.Query != "" {
		t.Errorf("query = %q, want empty", got.Query)
	}

	// An explicit zero offset is a real value and must be sent.
	c2, got2 := newTestServer(t, 200, `{"repositories":[],"total":0}`)
	if _, err := c2.ListRepositories(context.Background(), &Page{Offset: Int(0)}); err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if got2.Query != "offset=0" {
		t.Errorf("query = %q, want offset=0", got2.Query)
	}
}
