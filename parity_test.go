package credda

import (
	"context"
	"net/http"
	"testing"
)

// TestParityRequestShapes asserts the method + path + query + body + auth of the
// surfaces brought up to parity with the TypeScript SDK
// (packages/sdk/src/lib/client.ts) and the mounted routes
// (packages/api/src/app.ts). Query strings are url.Values.Encode() output
// (alphabetically sorted), matching what the client sends.
func TestParityRequestShapes(t *testing.T) {
	scoreMin := 40.0
	scoreMax := 90.0
	hasVerified := true
	minVerified := 2
	limit := 50
	notScored := false
	notFrozen := false
	ttl := 3600
	recent := 5
	meterDays := 30
	analyticsDays := 90

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
		// ── Benchmarks ────────────────────────────────────────────────────────
		{
			name:     "GetBenchmarks",
			response: `{"benchmarkVersion":"1","kAnonymity":{"minimumCohortSize":20}}`,
			call: func(c *Client) error {
				_, err := c.GetBenchmarks(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/benchmarks",
			wantNoAuth: true,
		},
		{
			name:     "GetBenchmarkDistribution with dimension and cohort",
			response: `{"available":true,"cohort":"gig","statistics":{"median":61}}`,
			call: func(c *Client) error {
				_, err := c.GetBenchmarkDistribution(context.Background(), "platform_category", "gig")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/benchmarks/distribution",
			wantQuery:  "cohort=gig&dimension=platform_category",
		},
		{
			name:     "GetBenchmarkDistribution whole dimension",
			response: `{"dimension":"all","populationSize":3,"cohorts":[]}`,
			call: func(c *Client) error {
				_, err := c.GetBenchmarkDistribution(context.Background(), "", "")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/benchmarks/distribution",
			wantQuery:  "",
		},
		{
			name:     "GetUserBenchmark",
			response: `{"userId":"u1","available":true,"percentile":72}`,
			call: func(c *Client) error {
				_, err := c.GetUserBenchmark(context.Background(), "u1", "platform_category")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/benchmark",
			wantQuery:  "dimension=platform_category",
		},

		// ── Book query / export ─────────────────────────────────────────────────
		{
			name:     "ListUsers with the full filter set",
			response: `{"data":[],"count":0,"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListUsers(context.Background(), &ListUsersQuery{
					BookFilterQuery: BookFilterQuery{
						ScoreMin:          &scoreMin,
						ScoreMax:          &scoreMax,
						Band:              "Good",
						SubjectType:       "PERSON",
						ActiveSince:       "2026-01-01T00:00:00Z",
						HasVerifiedEvents: &hasVerified,
						MinVerifiedEvents: &minVerified,
					},
					Sort:   "score",
					Order:  "desc",
					Limit:  &limit,
					Cursor: "cur_1",
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users",
			wantQuery:  "activeSince=2026-01-01T00%3A00%3A00Z&band=Good&cursor=cur_1&hasVerifiedEvents=true&limit=50&minVerifiedEvents=2&order=desc&scoreMax=90&scoreMin=40&sort=score&subjectType=PERSON",
		},
		{
			name:     "ListUsers with no filters",
			response: `{"data":[],"count":0,"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListUsers(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users",
			wantQuery:  "",
		},
		{
			// `false` is a filter VALUE, not an absent one — it must reach the wire.
			name:     "ListUsers sends the additional filters including the false cases",
			response: `{"data":[],"count":0,"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListUsers(context.Background(), &ListUsersQuery{
					BookFilterQuery: BookFilterQuery{
						HasScore:         &notScored,
						ScoreFrozen:      &notFrozen,
						SubjectType:      "ORGANIZATION",
						RegisteredSince:  "2026-01-01T00:00:00Z",
						RegisteredBefore: "2026-07-01T00:00:00Z",
					},
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users",
			wantQuery:  "hasScore=false&registeredBefore=2026-07-01T00%3A00%3A00Z&registeredSince=2026-01-01T00%3A00%3A00Z&scoreFrozen=false&subjectType=ORGANIZATION",
		},
		{
			name:     "GetBookSummary takes the same closed filter set",
			response: `{"formulaVersion":"5.3","matched":1284,"scored":1190,"unscored":94,"central":{"median":61.4,"mean":58.77},"bandDistribution":[{"band":"Excellent","minScore":80,"count":214,"share":17.98}]}`,
			call: func(c *Client) error {
				_, err := c.GetBookSummary(context.Background(), &BookFilterQuery{
					SubjectType:       "ORGANIZATION",
					HasVerifiedEvents: &hasVerified,
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/summary",
			wantQuery:  "hasVerifiedEvents=true&subjectType=ORGANIZATION",
		},
		{
			name:     "GetBookSummary with no filters",
			response: `{"formulaVersion":"5.3","matched":0,"scored":0,"unscored":0,"central":{"median":null,"mean":null},"bandDistribution":[]}`,
			call: func(c *Client) error {
				_, err := c.GetBookSummary(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/summary",
			wantQuery:  "",
		},

		// ── Trust summary ───────────────────────────────────────────────────────
		{
			name:     "GetTrustSummary without narrative",
			response: `{"userId":"u1","available":true,"summary":"..."}`,
			call: func(c *Client) error {
				_, err := c.GetTrustSummary(context.Background(), "u1", false)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/trust-summary",
			wantQuery:  "",
		},
		{
			name:     "GetTrustSummary with narrative",
			response: `{"userId":"u1","available":true}`,
			call: func(c *Client) error {
				_, err := c.GetTrustSummary(context.Background(), "u1", true)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/trust-summary",
			wantQuery:  "narrative=1",
		},

		// ── Wallet issuance ─────────────────────────────────────────────────────
		{
			name:     "GetCredentialIssuerMetadata",
			response: `{"credential_issuer":"https://api.credda.io"}`,
			call: func(c *Client) error {
				_, err := c.GetCredentialIssuerMetadata(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/.well-known/openid-credential-issuer",
			wantNoAuth: true,
		},
		{
			name:     "CreateCredentialOffer with options",
			response: `{"credentialOfferUri":"openid-credential-offer://?x","scope":"band"}`,
			call: func(c *Client) error {
				_, err := c.CreateCredentialOffer(context.Background(), "u1", CredentialOfferInput{
					CredentialConfigurationIDs: []string{"trust_dc_sd_jwt"},
					Scope:                      "band",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/credential-offer",
			wantBody:   `{"credentialConfigurationIds":["trust_dc_sd_jwt"],"scope":"band"}`,
		},
		{
			name:     "CreateCredentialOffer defaults to an empty body",
			response: `{"credentialOfferUri":"openid-credential-offer://?x"}`,
			call: func(c *Client) error {
				_, err := c.CreateCredentialOffer(context.Background(), "u1", CredentialOfferInput{})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/credential-offer",
			wantBody:   `{}`,
		},

		// ── Webhook replay ──────────────────────────────────────────────────────
		{
			name:     "ReplayWebhookDelivery",
			response: `{"status":"replayed","success":true,"statusCode":200,"error":null}`,
			call: func(c *Client) error {
				_, err := c.ReplayWebhookDelivery(context.Background(), "wh_1", "del_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/webhooks/wh_1/deliveries/del_1/replay",
			wantBody:   `{}`,
		},

		// ── Confirmation requests ───────────────────────────────────────────────
		{
			name:     "CreateConfirmationRequest",
			response: `{"confirmation":{"id":"cf_1","status":"PENDING"},"confirmationToken":"tok"}`,
			call: func(c *Client) error {
				_, err := c.CreateConfirmationRequest(context.Background(), CreateConfirmationInput{
					UserID:          "u1",
					EventType:       "CONTRACT_FULFILLED",
					CounterpartyRef: "client@example.com",
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/confirmations",
			wantBody:   `{"userId":"u1","eventType":"CONTRACT_FULFILLED","counterpartyRef":"client@example.com"}`,
		},
		{
			// api #296 — the hosted-page redirect travels on the create body.
			name:     "CreateConfirmationRequest with a hosted-page returnUrl",
			response: `{"confirmation":{"id":"cf_1"},"confirmUrl":"https://api.credda.io/confirm/cf_1?token=tok"}`,
			call: func(c *Client) error {
				_, err := c.CreateConfirmationRequest(context.Background(), CreateConfirmationInput{
					UserID:          "u1",
					EventType:       "CONTRACT_FULFILLED",
					CounterpartyRef: "client@example.com",
					ReturnURL:       "https://acme.example.com/orders/42",
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/confirmations",
			wantBody:   `{"userId":"u1","eventType":"CONTRACT_FULFILLED","counterpartyRef":"client@example.com","returnUrl":"https://acme.example.com/orders/42"}`,
		},
		{
			name:     "ListConfirmations filtered by status",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListConfirmations(context.Background(), "PENDING", &limit, "c1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/confirmations",
			wantQuery:  "cursor=c1&limit=50&status=PENDING",
		},
		{
			name:     "GetConfirmation",
			response: `{"confirmation":{"id":"cf_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetConfirmation(context.Background(), "cf_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/confirmations/cf_1",
		},
		{
			name:     "CancelConfirmation",
			response: `{"confirmation":{"id":"cf_1","status":"CANCELLED"}}`,
			call: func(c *Client) error {
				_, err := c.CancelConfirmation(context.Background(), "cf_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/confirmations/cf_1/cancel",
			wantBody:   `{}`,
		},
		{
			name:     "CreateConfirmationBatch posts the requests array with an idempotency key",
			response: `{"total":1,"created":1,"failed":0,"results":[]}`,
			call: func(c *Client) error {
				_, err := c.CreateConfirmationBatch(context.Background(), []CreateConfirmationInput{
					{UserID: "u1", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "client@example.com"},
				}, "book-warm-1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/confirmations/batch",
			wantBody:   `{"requests":[{"userId":"u1","eventType":"CONTRACT_FULFILLED","counterpartyRef":"client@example.com"}]}`,
		},
		{
			name:     "PreviewConfirmation is token-gated and keyless",
			response: `{"confirmation":{"id":"cf_1","platform":"Acme"}}`,
			call: func(c *Client) error {
				_, err := c.PreviewConfirmation(context.Background(), "cf_1", "tok_abc")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/confirmations/cf_1/preview",
			wantQuery:  "token=tok_abc",
			wantNoAuth: true,
		},
		{
			name:     "RespondToConfirmation is keyless",
			response: `{"status":"confirmed","confirmation":{"id":"cf_1"},"eventId":"ev_1"}`,
			call: func(c *Client) error {
				_, err := c.RespondToConfirmation(context.Background(), "cf_1", "tok_abc", "confirm")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/confirmations/cf_1/respond",
			wantBody:   `{"decision":"confirm","token":"tok_abc"}`,
			wantNoAuth: true,
		},

		// ── Reference / employment-verification requests ────────────────────────
		{
			name:     "CreateReferenceRequest",
			response: `{"reference":{"id":"rf_1","status":"PENDING"},"referenceToken":"tok"}`,
			call: func(c *Client) error {
				_, err := c.CreateReferenceRequest(context.Background(), CreateReferenceInput{
					UserID:          "u1",
					Category:        "employment",
					Label:           "Senior Engineer",
					CounterpartyRef: "manager@example.com",
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/references",
			wantBody:   `{"userId":"u1","category":"employment","label":"Senior Engineer","counterpartyRef":"manager@example.com"}`,
		},
		{
			name:     "CreateReferenceRequest with a hosted-page returnUrl",
			response: `{"reference":{"id":"rf_1"},"referenceUrl":"https://api.credda.io/reference/rf_1?token=tok"}`,
			call: func(c *Client) error {
				_, err := c.CreateReferenceRequest(context.Background(), CreateReferenceInput{
					UserID:          "u1",
					Category:        "education",
					CounterpartyRef: "registrar@example.edu",
					ReturnURL:       "https://acme.example.com/onboarding/42",
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/references",
			wantBody:   `{"userId":"u1","category":"education","counterpartyRef":"registrar@example.edu","returnUrl":"https://acme.example.com/onboarding/42"}`,
		},
		{
			name:     "ListReferences filtered by status",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListReferences(context.Background(), "PENDING", &limit, "c1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/references",
			wantQuery:  "cursor=c1&limit=50&status=PENDING",
		},
		{
			name:     "GetReference",
			response: `{"reference":{"id":"rf_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetReference(context.Background(), "rf_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/references/rf_1",
		},
		{
			name:     "CancelReference",
			response: `{"reference":{"id":"rf_1","status":"CANCELLED"}}`,
			call: func(c *Client) error {
				_, err := c.CancelReference(context.Background(), "rf_1")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/references/rf_1/cancel",
			wantBody:   `{}`,
		},
		{
			name:     "PreviewReference is token-gated and keyless",
			response: `{"reference":{"id":"rf_1","platform":"Acme"}}`,
			call: func(c *Client) error {
				_, err := c.PreviewReference(context.Background(), "rf_1", "tok_abc")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/references/rf_1/preview",
			wantQuery:  "token=tok_abc",
			wantNoAuth: true,
		},
		{
			name:     "RespondToReference is keyless",
			response: `{"status":"CONFIRMED","reference":{"id":"rf_1"},"eventId":"ev_1"}`,
			call: func(c *Client) error {
				_, err := c.RespondToReference(context.Background(), "rf_1", "tok_abc", "confirm")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/references/rf_1/respond",
			wantBody:   `{"decision":"confirm","token":"tok_abc"}`,
			wantNoAuth: true,
		},

		// ── Threshold policies ──────────────────────────────────────────────────
		{
			name:     "CreatePolicy",
			response: `{"policy":{"id":"pol_1","metric":"score"}}`,
			call: func(c *Client) error {
				threshold := 40.0
				_, err := c.CreatePolicy(context.Background(), CreatePolicyInput{
					Name:      "watch",
					UserID:    "u1",
					Metric:    "score",
					Direction: "down",
					Threshold: &threshold,
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/policies",
			wantBody:   `{"name":"watch","userId":"u1","metric":"score","direction":"down","threshold":40}`,
		},
		{
			name:     "ListPolicies",
			response: `{"data":[],"nextCursor":null}`,
			call: func(c *Client) error {
				_, err := c.ListPolicies(context.Background(), &limit, "p1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/policies",
			wantQuery:  "cursor=p1&limit=50",
		},
		{
			name:     "GetPolicy",
			response: `{"policy":{"id":"pol_1"}}`,
			call: func(c *Client) error {
				_, err := c.GetPolicy(context.Background(), "pol_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/policies/pol_1",
		},
		{
			name:     "UpdatePolicy",
			response: `{"policy":{"id":"pol_1","isActive":false}}`,
			call: func(c *Client) error {
				active := false
				_, err := c.UpdatePolicy(context.Background(), "pol_1", UpdatePolicyInput{IsActive: &active})
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/api/v1/policies/pol_1",
			wantBody:   `{"isActive":false}`,
		},
		{
			name:     "DeletePolicy",
			response: ``,
			call: func(c *Client) error {
				return c.DeletePolicy(context.Background(), "pol_1")
			},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/policies/pol_1",
		},

		// ── Open Badges ─────────────────────────────────────────────────────────
		{
			name:     "GetOpenBadgeAchievements",
			response: `{"achievementIds":["one-year-reliable"],"achievements":[]}`,
			call: func(c *Client) error {
				_, err := c.GetOpenBadgeAchievements(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/open-badges/achievements",
			wantNoAuth: true,
		},
		{
			name:     "GetOpenBadgeAchievement by id",
			response: `{"id":"https://api.credda.io/api/v1/open-badges/achievements/x"}`,
			call: func(c *Client) error {
				_, err := c.GetOpenBadgeAchievement(context.Background(), "x")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/open-badges/achievements/x",
			wantNoAuth: true,
		},

		// ── API version contract / changelog ────────────────────────────────────
		{
			name:     "GetChangelog is public",
			response: `{"apiVersion":"v1","deprecations":[],"count":1,"entries":[]}`,
			call: func(c *Client) error {
				_, err := c.GetChangelog(context.Background())
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/changelog",
			wantNoAuth: true,
		},

		// ── Verified Profile (qualifications) ───────────────────────────────────
		{
			name:     "GetVerifiedProfile",
			response: `{"userId":"u1","profileVersion":"1.0","verificationDepth":0.5}`,
			call: func(c *Client) error {
				_, err := c.GetVerifiedProfile(context.Background(), "u1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/verified-profile",
		},
		{
			name:     "RecordQualification posts the claim with its witness",
			response: `{"userId":"u1","eventId":"ev_1","isVerified":true,"verificationNote":null}`,
			call: func(c *Client) error {
				_, err := c.RecordQualification(context.Background(), "u1", RecordQualificationInput{
					Category:   "certification",
					Label:      "CCNA",
					Issuer:     "Cisco",
					VerifiedBy: "registrar@cisco.example",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/qualifications",
			wantBody:   `{"category":"certification","label":"CCNA","issuer":"Cisco","verifiedBy":"registrar@cisco.example"}`,
		},
		{
			// The witness rule decides verification — the SDK can never ask for it.
			name:     "RecordQualification without a witness sends no verified flag",
			response: `{"userId":"u1","eventId":"ev_2","isVerified":false}`,
			call: func(c *Client) error {
				_, err := c.RecordQualification(context.Background(), "u1", RecordQualificationInput{
					Category: "skill",
					Label:    "Rust",
				})
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/qualifications",
			wantBody:   `{"category":"skill","label":"Rust"}`,
		},

		// ── Professional Record ─────────────────────────────────────────────────
		{
			name:     "GetProfessionalRecord",
			response: `{"userId":"u1","professionalRecordVersion":"1.0"}`,
			call: func(c *Client) error {
				_, err := c.GetProfessionalRecord(context.Background(), "u1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/professional-record",
		},
		{
			name:     "MintProfessionalRecordCredential posts an empty body by default",
			response: `{"credentialVc":"ey...","credentialType":"CreddaProfessionalRecordCredential"}`,
			call: func(c *Client) error {
				_, err := c.MintProfessionalRecordCredential(context.Background(), "u1", nil)
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/professional-record/credential",
			wantBody:   `{}`,
		},
		{
			name:     "MintProfessionalRecordCredential forwards ttlSeconds",
			response: `{"credentialVc":"ey..."}`,
			call: func(c *Client) error {
				_, err := c.MintProfessionalRecordCredential(context.Background(), "u1", &ttl)
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/users/u1/professional-record/credential",
			wantBody:   `{"ttlSeconds":3600}`,
		},
		{
			// The token is the capability — an API key must never be sent here,
			// and the record block is served ONLY at full disclosure.
			name:     "GetPublicProfessionalRecord is keyless and asks for full scope",
			response: `{"token":"tok","scope":"full","professionalRecord":{"professionalRecordVersion":"1.0"}}`,
			call: func(c *Client) error {
				_, err := c.GetPublicProfessionalRecord(context.Background(), "tok_abc")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_abc",
			wantQuery:  "professional=1&scope=full",
			wantNoAuth: true,
		},

		// ── Worker reliability report ───────────────────────────────────────────
		{
			name:     "GetReliabilityReport reads the dossier with the key",
			response: `{"userId":"u1","reliabilityReportVersion":"1.0","reliability":{"band":"Good"}}`,
			call: func(c *Client) error {
				_, err := c.GetReliabilityReport(context.Background(), "u1", nil, false)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/reliability-report",
		},
		{
			name:     "GetReliabilityReport forwards recent + benchmark",
			response: `{"userId":"u1","reliabilityReportVersion":"1.0"}`,
			call: func(c *Client) error {
				_, err := c.GetReliabilityReport(context.Background(), "u1", &recent, true)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/u1/reliability-report",
			wantQuery:  "benchmark=1&recent=5",
		},
		{
			// The token is the capability — an API key must never be sent here.
			name:     "GetPublicReliabilityReport is keyless",
			response: `{"token":"tok","issuer":"credda.io","reliabilityReport":{"reliabilityReportVersion":"1.0"}}`,
			call: func(c *Client) error {
				_, err := c.GetPublicReliabilityReport(context.Background(), "tok_abc", nil, false)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_abc/reliability-report",
			wantNoAuth: true,
		},

		// ── Career export ───────────────────────────────────────────────────────
		{
			name:     "GetCareerExport is keyed",
			response: `{"$schema":"https://jsonresume.org/schema","work":[{"name":"Acme","credda":{"verified":true}}],"meta":{}}`,
			call: func(c *Client) error {
				_, err := c.GetCareerExport(context.Background(), "ext-1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/ext-1/career-export",
		},
		{
			// The token IS the consent — a public route must never carry a key.
			name:     "GetPublicCareerExport is keyless",
			response: `{"$schema":"https://jsonresume.org/schema","meta":{}}`,
			call: func(c *Client) error {
				_, err := c.GetPublicCareerExport(context.Background(), "tok_abc")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/verify/tok_abc/career-export",
			wantNoAuth: true,
		},

		// ── Outcome templates ───────────────────────────────────────────────────
		{
			name:     "GetOutcomeTemplates is public, no filter",
			response: `{"version":"1.0","industries":[],"templates":[]}`,
			call: func(c *Client) error {
				_, err := c.GetOutcomeTemplates(context.Background(), "")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/outcome-templates",
			wantNoAuth: true,
		},
		{
			name:     "GetOutcomeTemplates filters by industry, still keyless",
			response: `{"version":"1.0","industries":[],"templates":[]}`,
			call: func(c *Client) error {
				_, err := c.GetOutcomeTemplates(context.Background(), "trades")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/outcome-templates",
			wantQuery:  "industry=trades",
			wantNoAuth: true,
		},

		// ── Dispatch reliability ────────────────────────────────────────────────
		{
			// The context param is pinned by the method, not caller-supplied.
			name:     "GetDispatchReliability pins context=dispatch",
			response: `{"userId":"w1","context":"dispatch","dispatchReliabilityVersion":"1.0","score":72.5,"evidence":{"totalOutcomes":18,"verifiedOutcomes":12},"topFactors":[]}`,
			call: func(c *Client) error {
				_, err := c.GetDispatchReliability(context.Background(), "w1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/users/w1/reliability",
			wantQuery:  "context=dispatch",
		},

		// ── Usage meters ────────────────────────────────────────────────────────
		{
			name:     "GetUsageMeters with no window",
			response: `{"platform":{"id":"p1"},"from":"2026-07-18","to":"2026-07-24","meters":[]}`,
			call: func(c *Client) error {
				_, err := c.GetUsageMeters(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/usage/meters",
		},
		{
			name:     "GetUsageMeters with a trailing-days window",
			response: `{"platform":{"id":"p1"},"from":"2026-06-25","to":"2026-07-24","meters":[]}`,
			call: func(c *Client) error {
				_, err := c.GetUsageMeters(context.Background(), &AnalyticsWindow{Days: &meterDays})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/usage/meters",
			wantQuery:  "days=30",
		},
		{
			name:     "GetUsageMeters with an explicit range",
			response: `{"platform":{"id":"p1"},"from":"2026-06-01","to":"2026-06-30","meters":[]}`,
			call: func(c *Client) error {
				_, err := c.GetUsageMeters(context.Background(), &AnalyticsWindow{From: "2026-06-01", To: "2026-06-30"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/usage/meters",
			wantQuery:  "from=2026-06-01&to=2026-06-30",
		},

		// ── Platform analytics (aggregate-only) ─────────────────────────────────
		{
			name:     "GetEventAnalytics with no window",
			response: `{"window":{},"totals":{"total":0,"verified":0,"verifiedShare":null},"daily":[],"byType":[]}`,
			call: func(c *Client) error {
				_, err := c.GetEventAnalytics(context.Background(), nil)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/analytics/events",
		},
		{
			name:     "GetEventAnalytics with a trailing-days window",
			response: `{"window":{"days":90},"totals":{"total":3,"verified":2,"verifiedShare":0.667},"daily":[],"byType":[]}`,
			call: func(c *Client) error {
				_, err := c.GetEventAnalytics(context.Background(), &AnalyticsWindow{Days: &analyticsDays})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/analytics/events",
			wantQuery:  "days=90",
		},
		{
			name:     "GetScoreAnalytics with an explicit range",
			response: `{"formulaVersion":"5.3","window":{},"scoredSubjects":0,"central":{"median":null,"mean":null},"bandDistribution":[],"movement":{}}`,
			call: func(c *Client) error {
				_, err := c.GetScoreAnalytics(context.Background(), &AnalyticsWindow{From: "2026-01-01", To: "2026-03-31"})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/analytics/scores",
			wantQuery:  "from=2026-01-01&to=2026-03-31",
		},

		// ── Activation campaigns ────────────────────────────────────────────────
		{
			// rowKey rides inline via the embedded CreateConfirmationInput.
			name:     "CreateActivationCampaign posts name + rows",
			response: `{"campaign":{"id":"cmp_1","name":"March roster","submittedCount":1,"createdAt":"2026-07-24T10:00:00.000Z"},"created":1,"failed":0,"duplicates":[],"results":[]}`,
			call: func(c *Client) error {
				_, err := c.CreateActivationCampaign(context.Background(), CreateActivationCampaignInput{
					Name: "March roster",
					Rows: []ActivationRow{{
						CreateConfirmationInput: CreateConfirmationInput{
							UserID: "u1", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "c1",
						},
						RowKey: "shift_9",
					}},
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/activation/campaigns",
			wantBody:   `{"name":"March roster","rows":[{"userId":"u1","eventType":"CONTRACT_FULFILLED","counterpartyRef":"c1","rowKey":"shift_9"}]}`,
		},
		{
			name:     "CreateActivationCampaign omits an empty name",
			response: `{"campaign":{"id":"cmp_2"},"created":1,"failed":0,"duplicates":[],"results":[]}`,
			call: func(c *Client) error {
				_, err := c.CreateActivationCampaign(context.Background(), CreateActivationCampaignInput{
					Rows: []ActivationRow{{CreateConfirmationInput: CreateConfirmationInput{
						UserID: "u1", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "c1",
					}}},
				}, "")
				return err
			},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/activation/campaigns",
			wantBody:   `{"rows":[{"userId":"u1","eventType":"CONTRACT_FULFILLED","counterpartyRef":"c1"}]}`,
		},
		{
			name:     "GetActivationCampaign reads the funnel",
			response: `{"campaign":{"id":"cmp_1","name":null,"submittedCount":10,"createdAt":"2026-07-24T10:00:00.000Z"},"funnel":{"submitted":10,"pending":4,"confirmed":5,"declined":1,"expired":0,"cancelled":0,"confirmationRate":0.5}}`,
			call: func(c *Client) error {
				_, err := c.GetActivationCampaign(context.Background(), "cmp_1")
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/activation/campaigns/cmp_1",
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

// TestParityDecodesResponseBodies confirms the new payload structs unmarshal.
func TestParityDecodesResponseBodies(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"data":[{"externalId":"u1","finalScore":72,"scoreBand":"Good","eventCount":5,"verifiedEventCount":3,"registeredAt":"2026-01-01T00:00:00Z"}],"count":1,"nextCursor":null}`)
	out, err := c.ListUsers(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if out.Count != 1 || len(out.Data) != 1 {
		t.Fatalf("count/data = %d/%d, want 1/1", out.Count, len(out.Data))
	}
	if out.Data[0].ExternalID != "u1" || out.Data[0].VerifiedEventCount != 3 {
		t.Fatalf("subject decoded wrong: %+v", out.Data[0])
	}
	if out.NextCursor != nil {
		t.Fatalf("nextCursor = %v, want nil", out.NextCursor)
	}
}

// TestVerifiedProfileDecodesNullDepthAsNil pins the "null, not 0" invariant: a
// category with nothing claimed has no depth, and reporting 0 would read as
// "claimed but unverified" — a materially different (and wrong) statement.
func TestVerifiedProfileDecodesNullDepthAsNil(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"u1","profileVersion":"1.0",
		"categories":{
			"education":{"claimed":2,"verified":1,"verificationDepth":0.5},
			"skill":{"claimed":0,"verified":0,"verificationDepth":null}
		},
		"totals":{"claimed":2,"verified":1,"selfAttested":1},
		"verificationDepth":0.5,
		"disclosures":["not an assessment of the person"]
	}`)
	out, err := c.GetVerifiedProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetVerifiedProfile: %v", err)
	}
	if out.VerificationDepth == nil || *out.VerificationDepth != 0.5 {
		t.Fatalf("verificationDepth = %v, want 0.5", out.VerificationDepth)
	}
	if d := out.Categories["skill"].VerificationDepth; d != nil {
		t.Fatalf("skill depth = %v, want nil (nothing claimed)", *d)
	}
	if out.Totals.SelfAttested != 1 {
		t.Fatalf("selfAttested = %d, want 1", out.Totals.SelfAttested)
	}
	if len(out.Disclosures) != 1 {
		t.Fatalf("disclosures = %v, want one entry", out.Disclosures)
	}
}

// TestProfessionalRecordDecodes covers the embedded-struct payload (the record
// plus userId) and the fail-safe null block on the public verify variant.
func TestProfessionalRecordDecodes(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"u1",
		"professionalRecordVersion":"1.0",
		"reliability":{"score":72,"band":"Good","confidence":0.8},
		"verifiedExperience":{"verifiedOutcomes":9,"totalOutcomes":12,"verificationDepth":0.75,"verifiedPlatforms":2},
		"tenure":{"firstRecordedAt":"2026-01-01T00:00:00Z","trackRecordDays":400,"trackRecordMonths":13.1},
		"status":{"scoreFrozen":false},
		"provenance":{"formulaVersion":"5.3","computedAt":null},
		"disclosures":["not a background check"]
	}`)
	out, err := c.GetProfessionalRecord(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetProfessionalRecord: %v", err)
	}
	if out.UserID != "u1" || out.ProfessionalRecordVersion != "1.0" {
		t.Fatalf("embedded record decoded wrong: %+v", out)
	}
	if out.Reliability.Band == nil || *out.Reliability.Band != "Good" ||
		out.VerifiedExperience.VerifiedOutcomes != 9 {
		t.Fatalf("record fields decoded wrong: %+v", out.ProfessionalRecord)
	}
	if out.Tenure.TrackRecordDays == nil || *out.Tenure.TrackRecordDays != 400 {
		t.Fatalf("trackRecordDays = %v, want 400", out.Tenure.TrackRecordDays)
	}
	// A missing figure stays nil — never a default.
	if out.Provenance.ComputedAt != nil {
		t.Fatalf("computedAt = %v, want nil", *out.Provenance.ComputedAt)
	}
}

func TestPublicProfessionalRecordToleratesANullBlock(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"token":"tok","scope":"full","finalScore":72,"professionalRecord":null}`)
	out, err := c.GetPublicProfessionalRecord(context.Background(), "tok")
	if err != nil {
		t.Fatalf("GetPublicProfessionalRecord: %v", err)
	}
	if out.ProfessionalRecord != nil {
		t.Fatalf("professionalRecord = %+v, want nil (fail-safe)", out.ProfessionalRecord)
	}
	// The embedded public trust payload still decodes.
	if out.Token != "tok" || out.FinalScore == nil || *out.FinalScore != 72 {
		t.Fatalf("embedded TrustPayload decoded wrong: %+v", out.TrustPayload)
	}
}

// TestReliabilityReportDecodes covers the keyed dossier (embedded record + userId)
// and the self/verified flag on recent outcomes.
func TestReliabilityReportDecodes(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"worker_7",
		"reliabilityReportVersion":"1.0",
		"reliability":{"score":72,"band":"Good","confidence":0.8,"formulaVersion":"5.3","reasonCodesVersion":"1.0"},
		"metrics":{"completionRate":0.95,"onTimeRate":0.9,"consistency":0.8,"recency":null,"disputeRate":0},
		"verifiedExperience":{"verifiedOutcomes":34,"totalOutcomes":40,"verificationDepth":0.85,"verifiedPlatforms":3,"tenure":{"trackRecordDays":541}},
		"topFactors":[{"code":"ESTABLISHED_VERIFIED_HISTORY","factor":"evidence","direction":"supporting","rank":1,"contribution":0.4}],
		"recentOutcomes":[
			{"eventType":"CONTRACT_FULFILLED","stake":"HIGH","verified":true,"source":"verified","occurredAt":"2026-07-10T00:00:00Z"},
			{"eventType":"TRANSACTION_COMPLETED","stake":"LOW","verified":false,"source":"self_reported","occurredAt":"2026-07-01T00:00:00Z"}
		],
		"benchmark":null,
		"status":{"scoreFrozen":false},
		"provenance":{"formulaVersion":"5.3","computedAt":"2026-07-10T00:00:00Z"},
		"disclosures":["not a background check"],
		"advisory":"aggregates values the score already produced"
	}`)
	out, err := c.GetReliabilityReport(context.Background(), "worker_7", nil, false)
	if err != nil {
		t.Fatalf("GetReliabilityReport: %v", err)
	}
	if out.UserID != "worker_7" || out.Reliability.Band == nil || *out.Reliability.Band != "Good" {
		t.Fatalf("report decoded wrong: %+v", out)
	}
	// recency is nil (not 0) when there is no dated activity.
	if out.Metrics.Recency != nil {
		t.Fatalf("recency = %v, want nil", *out.Metrics.Recency)
	}
	if out.Benchmark != nil {
		t.Fatalf("benchmark = %+v, want nil", out.Benchmark)
	}
	// self-reported vs verified must survive decoding unmissably.
	self := out.RecentOutcomes[1]
	if self.Source != "self_reported" || self.Verified {
		t.Fatalf("self-reported outcome decoded wrong: %+v", self)
	}
	if out.VerifiedExperience.Tenure.TrackRecordDays == nil || *out.VerifiedExperience.Tenure.TrackRecordDays != 541 {
		t.Fatalf("tenure decoded wrong: %+v", out.VerifiedExperience.Tenure)
	}
}

func TestPublicReliabilityReportToleratesANullBlock(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{"token":"tok","issuer":"credda.io","reliabilityReport":null}`)
	out, err := c.GetPublicReliabilityReport(context.Background(), "tok", nil, false)
	if err != nil {
		t.Fatalf("GetPublicReliabilityReport: %v", err)
	}
	if out.ReliabilityReport != nil {
		t.Fatalf("reliabilityReport = %+v, want nil (fail-safe)", out.ReliabilityReport)
	}
	if out.Token != "tok" {
		t.Fatalf("token = %q, want tok", out.Token)
	}
}

// TestConfirmationBatchResultDecodes covers partial success — an ok item carries
// its token, a failed one its code, and neither writes to the ledger.
func TestConfirmationBatchResultDecodes(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"total":2,"created":1,"failed":1,
		"results":[
			{"index":0,"ok":true,"userId":"u1","id":"cf_1","status":"PENDING","confirmationToken":"tok_1","confirmUrl":"https://api.credda.io/confirm/cf_1?token=tok_1"},
			{"index":1,"ok":false,"userId":"u2","error":"cannot confirm your own outcome","code":"CONFIRMATION_SELF"}
		]
	}`)
	out, err := c.CreateConfirmationBatch(context.Background(), []CreateConfirmationInput{
		{UserID: "u1", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "c1"},
		{UserID: "u2", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "u2"},
	}, "book-warm-1")
	if err != nil {
		t.Fatalf("CreateConfirmationBatch: %v", err)
	}
	if out.Created != 1 || out.Failed != 1 || len(out.Results) != 2 {
		t.Fatalf("batch totals decoded wrong: %+v", out)
	}
	if out.Results[0].ConfirmationToken != "tok_1" {
		t.Fatalf("ok item token = %q", out.Results[0].ConfirmationToken)
	}
	if out.Results[1].Code != "CONFIRMATION_SELF" {
		t.Fatalf("failed item code = %q", out.Results[1].Code)
	}
}

// TestConfirmationHostedFieldsDecode covers api #296's confirmUrl / returnUrl.
func TestConfirmationHostedFieldsDecode(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"confirmation":{"id":"cf_1","status":"PENDING","returnUrl":"https://acme.example.com/orders/42"},
		"confirmationToken":"tok",
		"confirmUrl":"https://api.credda.io/confirm/cf_1?token=tok",
		"previewUrl":"https://api.credda.io/api/v1/confirmations/cf_1/preview?token=tok",
		"respondUrl":"https://api.credda.io/api/v1/confirmations/cf_1/respond"
	}`)
	out, err := c.CreateConfirmationRequest(context.Background(), CreateConfirmationInput{
		UserID: "u1", EventType: "CONTRACT_FULFILLED", CounterpartyRef: "c1",
	}, "")
	if err != nil {
		t.Fatalf("CreateConfirmationRequest: %v", err)
	}
	if out.ConfirmURL != "https://api.credda.io/confirm/cf_1?token=tok" {
		t.Fatalf("confirmUrl = %q", out.ConfirmURL)
	}
	if out.Confirmation.ReturnURL == nil || *out.Confirmation.ReturnURL != "https://acme.example.com/orders/42" {
		t.Fatalf("returnUrl = %v", out.Confirmation.ReturnURL)
	}
}

// TestParityKeyedCallsRejectedLocally confirms the keyed parity methods refuse to
// send without a key (the client rejects locally before any request).
func TestParityKeyedCallsRejectedLocally(t *testing.T) {
	c := NewClient(WithBaseURL("https://api.test.credda.io")) // no key
	ctx := context.Background()
	calls := map[string]func() error{
		"GetBenchmarkDistribution": func() error { _, e := c.GetBenchmarkDistribution(ctx, "", ""); return e },
		"GetUserBenchmark":         func() error { _, e := c.GetUserBenchmark(ctx, "u1", ""); return e },
		"ListUsers":                func() error { _, e := c.ListUsers(ctx, nil); return e },
		"GetTrustSummary":          func() error { _, e := c.GetTrustSummary(ctx, "u1", false); return e },
		"CreateCredentialOffer":    func() error { _, e := c.CreateCredentialOffer(ctx, "u1", CredentialOfferInput{}); return e },
		"CreateConfirmationRequest": func() error {
			_, e := c.CreateConfirmationRequest(ctx, CreateConfirmationInput{UserID: "u1"}, "")
			return e
		},
		"CreateConfirmationBatch": func() error {
			_, e := c.CreateConfirmationBatch(ctx, []CreateConfirmationInput{{UserID: "u1"}}, "")
			return e
		},
		"GetReliabilityReport":  func() error { _, e := c.GetReliabilityReport(ctx, "u1", nil, false); return e },
		"ListConfirmations":     func() error { _, e := c.ListConfirmations(ctx, "", nil, ""); return e },
		"CreatePolicy":          func() error { _, e := c.CreatePolicy(ctx, CreatePolicyInput{Name: "x"}); return e },
		"ListPolicies":          func() error { _, e := c.ListPolicies(ctx, nil, ""); return e },
		"ReplayWebhookDelivery": func() error { _, e := c.ReplayWebhookDelivery(ctx, "wh", "del"); return e },
		"GetVerifiedProfile":    func() error { _, e := c.GetVerifiedProfile(ctx, "u1"); return e },
		"RecordQualification": func() error {
			_, e := c.RecordQualification(ctx, "u1", RecordQualificationInput{Category: "skill"})
			return e
		},
		"GetProfessionalRecord": func() error { _, e := c.GetProfessionalRecord(ctx, "u1"); return e },
		"MintProfessionalRecordCredential": func() error {
			_, e := c.MintProfessionalRecordCredential(ctx, "u1", nil)
			return e
		},
		"GetDispatchReliability": func() error { _, e := c.GetDispatchReliability(ctx, "u1"); return e },
		"GetUsageMeters":         func() error { _, e := c.GetUsageMeters(ctx, nil); return e },
		"GetEventAnalytics":      func() error { _, e := c.GetEventAnalytics(ctx, nil); return e },
		"GetScoreAnalytics":      func() error { _, e := c.GetScoreAnalytics(ctx, nil); return e },
		"CreateActivationCampaign": func() error {
			_, e := c.CreateActivationCampaign(ctx, CreateActivationCampaignInput{
				Rows: []ActivationRow{{CreateConfirmationInput: CreateConfirmationInput{UserID: "u1"}}},
			}, "")
			return e
		},
		"GetActivationCampaign": func() error { _, e := c.GetActivationCampaign(ctx, "cmp_1"); return e },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s: expected an error without an API key", name)
		}
	}
}

// TestDispatchReliabilityDecodesNullAsNil pins the "null, not 0" invariant on the
// dispatch read: an unscored subject with an empty ledger has NO no-show rate,
// and reporting 0.0 would read as a perfect record — a materially different (and
// wrong) statement to make about someone before a shift.
func TestDispatchReliabilityDecodesNullAsNil(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"new_1","context":"dispatch","dispatchReliabilityVersion":"1.0",
		"score":null,"band":null,"confidence":null,"scoreFrozen":false,
		"formulaVersion":null,"computedAt":null,
		"evidence":{"totalOutcomes":0,"verifiedOutcomes":0},
		"noShowRate":null,"onTimeRate":null,"daysSinceLastEvent":null,
		"topFactors":[],"note":"","disclosures":[]
	}`)
	out, err := c.GetDispatchReliability(context.Background(), "new_1")
	if err != nil {
		t.Fatalf("GetDispatchReliability: %v", err)
	}
	if out.Score != nil {
		t.Fatalf("score = %v, want nil for an unscored subject", *out.Score)
	}
	if out.NoShowRate != nil {
		t.Fatalf("noShowRate = %v, want nil (absent) — never a placeholder 0", *out.NoShowRate)
	}
	if out.OnTimeRate != nil || out.DaysSinceLastEvent != nil {
		t.Fatalf("absent metrics decoded as values: %+v", out)
	}
	if out.Evidence.TotalOutcomes != 0 {
		t.Fatalf("evidence = %+v", out.Evidence)
	}
}

// TestDispatchReliabilityDecodesARealZero is the counterpart: a real 0.0 no-show
// rate earned over a real record must survive as a value, not collapse to nil.
func TestDispatchReliabilityDecodesARealZero(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"userId":"w1","context":"dispatch","dispatchReliabilityVersion":"1.0",
		"score":72.5,"band":"Good","confidence":0.8,"scoreFrozen":false,
		"evidence":{"totalOutcomes":18,"verifiedOutcomes":12},
		"noShowRate":0,"onTimeRate":0.94,"daysSinceLastEvent":3,
		"topFactors":[{"code":"OTR_STRONG","direction":"supporting","title":"On-time delivery","contribution":0.31}],
		"note":"Evidence, not a verdict.","disclosures":["Credda renders no decision."]
	}`)
	out, err := c.GetDispatchReliability(context.Background(), "w1")
	if err != nil {
		t.Fatalf("GetDispatchReliability: %v", err)
	}
	if out.NoShowRate == nil || *out.NoShowRate != 0 {
		t.Fatalf("noShowRate = %v, want a real 0", out.NoShowRate)
	}
	if len(out.TopFactors) != 1 || out.TopFactors[0].Direction != "supporting" {
		t.Fatalf("topFactors = %+v", out.TopFactors)
	}
}

// TestScoreAnalyticsDecodesNullCentralTendency: an empty book has no median or
// mean. Reporting 0 would be a claim about the population that is not true.
func TestScoreAnalyticsDecodesNullCentralTendency(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"formulaVersion":"5.3","window":{},"scoredSubjects":0,
		"central":{"median":null,"mean":null},"bandDistribution":[],
		"movement":{"up":0,"down":0,"unchanged":0,"subjectsMoved":0,"subjectsRecomputed":0}
	}`)
	out, err := c.GetScoreAnalytics(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetScoreAnalytics: %v", err)
	}
	if out.Central.Median != nil || out.Central.Mean != nil {
		t.Fatalf("central = %+v, want nils on an empty book", out.Central)
	}
	if out.FormulaVersion != "5.3" {
		t.Fatalf("formulaVersion = %q", out.FormulaVersion)
	}
}

// TestActivationCampaignDecodesPartialSuccess: a campaign is partial-success by
// design — an ok row carries a one-time token, a failed one carries a code, and
// NEITHER writes anything to the ledger.
func TestActivationCampaignDecodesPartialSuccess(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"campaign":{"id":"cmp_1","name":"March roster","submittedCount":3,"createdAt":"2026-07-24T10:00:00.000Z"},
		"created":1,"failed":1,
		"duplicates":[{"index":2,"rowKey":"shift_9"}],
		"results":[
			{"index":0,"ok":true,"userId":"u1","id":"cf_1","status":"PENDING","rowKey":"shift_9","confirmationToken":"tok","confirmUrl":"https://api.credda.io/confirm/cf_1?token=tok"},
			{"index":1,"ok":false,"userId":"u2","error":"self-confirmation","code":"CONFIRMATION_SELF"}
		]
	}`)
	out, err := c.CreateActivationCampaign(context.Background(), CreateActivationCampaignInput{
		Rows: []ActivationRow{{CreateConfirmationInput: CreateConfirmationInput{UserID: "u1"}}},
	}, "roster-2026-03")
	if err != nil {
		t.Fatalf("CreateActivationCampaign: %v", err)
	}
	if out.Campaign.Name == nil || *out.Campaign.Name != "March roster" {
		t.Fatalf("campaign name = %v", out.Campaign.Name)
	}
	if len(out.Duplicates) != 1 || out.Duplicates[0].RowKey != "shift_9" {
		t.Fatalf("duplicates = %+v", out.Duplicates)
	}
	if !out.Results[0].OK || out.Results[0].ConfirmationToken != "tok" {
		t.Fatalf("ok row = %+v", out.Results[0])
	}
	if out.Results[1].OK || out.Results[1].Code != "CONFIRMATION_SELF" {
		t.Fatalf("failed row = %+v", out.Results[1])
	}
}

// TestActivationFunnelDecodesFactualCounts: the funnel is counts, never a verdict.
func TestActivationFunnelDecodesFactualCounts(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"campaign":{"id":"cmp_1","name":null,"submittedCount":10,"createdAt":"2026-07-24T10:00:00.000Z"},
		"funnel":{"submitted":10,"pending":4,"confirmed":5,"declined":1,"expired":0,"cancelled":0,"confirmationRate":0.5}
	}`)
	out, err := c.GetActivationCampaign(context.Background(), "cmp_1")
	if err != nil {
		t.Fatalf("GetActivationCampaign: %v", err)
	}
	if out.Campaign.Name != nil {
		t.Fatalf("unnamed campaign should decode name as nil, got %q", *out.Campaign.Name)
	}
	if out.Funnel.ConfirmationRate != 0.5 || out.Funnel.Confirmed != 5 {
		t.Fatalf("funnel = %+v", out.Funnel)
	}
}

// TestUsageMetersDecodesTheBillingShape.
func TestUsageMetersDecodesTheBillingShape(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"platform":{"id":"p1","name":"Acme","tier":"GROWTH"},
		"window":{"days":7},"from":"2026-07-18","to":"2026-07-24",
		"meters":[
			{"meter":"credda_api_requests","dimension":"total","value":"all","quantity":412},
			{"meter":"credda_api_requests","dimension":"endpoint","value":"GET /api/v1/users/:id/score","quantity":300}
		]
	}`)
	out, err := c.GetUsageMeters(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetUsageMeters: %v", err)
	}
	if out.From != "2026-07-18" || out.To != "2026-07-24" {
		t.Fatalf("window bounds = %q..%q", out.From, out.To)
	}
	if len(out.Meters) != 2 || out.Meters[1].Dimension != "endpoint" {
		t.Fatalf("meters = %+v", out.Meters)
	}
}

// TestEventAnalyticsDecodesNullVerifiedShare: a bucket with no events has no
// verified share — nil, not 0 (0 would claim "events, none verified").
func TestEventAnalyticsDecodesNullVerifiedShare(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"window":{"days":30,"from":"2026-06-25","to":"2026-07-24"},
		"totals":{"total":4,"verified":3,"verifiedShare":0.75},
		"daily":[{"date":"2026-07-24","total":0,"verified":0,"verifiedShare":null}],
		"byType":[{"eventType":"CONTRACT_FULFILLED","total":4,"verified":3,"verifiedShare":0.75}]
	}`)
	out, err := c.GetEventAnalytics(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetEventAnalytics: %v", err)
	}
	if out.Daily[0].VerifiedShare != nil {
		t.Fatalf("empty day verifiedShare = %v, want nil", *out.Daily[0].VerifiedShare)
	}
	if out.ByType[0].VerifiedShare == nil || *out.ByType[0].VerifiedShare != 0.75 {
		t.Fatalf("byType verifiedShare = %v", out.ByType[0].VerifiedShare)
	}
}

// TestEventAnalyticsDecodesConfirmed pins the counterparty-confirmed dimension.
// `verified` on a directly-reported event is the reporting platform's own
// assertion; `confirmed` is the subset a distinct counterparty acted on. A struct
// that drops the field would silently report zero third-party evidence.
func TestEventAnalyticsDecodesConfirmed(t *testing.T) {
	c, _ := newTestServer(t, http.StatusOK, `{
		"window":{"days":30,"from":"2026-06-25","to":"2026-07-24"},
		"totals":{"total":4,"verified":3,"confirmed":1,"verifiedShare":75,"confirmedShare":25},
		"daily":[{"date":"2026-07-24","total":0,"verified":0,"confirmed":0,"verifiedShare":null,"confirmedShare":null}],
		"byType":[{"eventType":"CONTRACT_FULFILLED","total":4,"verified":3,"confirmed":1,"verifiedShare":75,"confirmedShare":25}]
	}`)
	out, err := c.GetEventAnalytics(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetEventAnalytics: %v", err)
	}
	if out.Totals.Confirmed != 1 {
		t.Fatalf("totals.confirmed = %d, want 1", out.Totals.Confirmed)
	}
	if out.Totals.ConfirmedShare == nil || *out.Totals.ConfirmedShare != 25 {
		t.Fatalf("totals.confirmedShare = %v, want 25", out.Totals.ConfirmedShare)
	}
	if out.Totals.Confirmed > out.Totals.Verified {
		t.Fatalf("confirmed (%d) must never exceed verified (%d)", out.Totals.Confirmed, out.Totals.Verified)
	}
	if out.Daily[0].ConfirmedShare != nil {
		t.Fatalf("empty day confirmedShare = %v, want nil", *out.Daily[0].ConfirmedShare)
	}
	if out.ByType[0].Confirmed != 1 {
		t.Fatalf("byType confirmed = %d, want 1", out.ByType[0].Confirmed)
	}
}

// TestCreateActivationCampaignSendsIdempotencyKey pins the retry header — a
// retried whole submission must replay, never mint a second set of tokens.
func TestCreateActivationCampaignSendsIdempotencyKey(t *testing.T) {
	c, got := newTestServer(t, http.StatusOK, `{"campaign":{"id":"cmp_1"},"created":0,"failed":0,"duplicates":[],"results":[]}`)
	if _, err := c.CreateActivationCampaign(context.Background(), CreateActivationCampaignInput{
		Rows: []ActivationRow{{CreateConfirmationInput: CreateConfirmationInput{UserID: "u1"}}},
	}, "roster-2026-03"); err != nil {
		t.Fatalf("CreateActivationCampaign: %v", err)
	}
	if v := got.Header.Get("Idempotency-Key"); v != "roster-2026-03" {
		t.Fatalf("Idempotency-Key = %q, want roster-2026-03", v)
	}
}
