package credda

import (
	"context"
	"net/url"
	"strings"
)

// ── Public (no API key) ─────────────────────────────────────────────────────

// ResolveToken resolves a public share token to a minimal, PII-free trust
// payload. GET /api/v1/verify/:token.
func (c *Client) ResolveToken(ctx context.Context, token string) (*TrustPayload, error) {
	var out TrustPayload
	if err := c.get(ctx, "/verify/"+esc(token), false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTrustExport fetches the portable, self-verifying trust export for a share
// token: current public score + history + a signed W3C credential + a
// revocation pointer. GET /api/v1/verify/:token/export.
func (c *Client) GetTrustExport(ctx context.Context, token string) (*TrustExport, error) {
	var out TrustExport
	if err := c.get(ctx, "/verify/"+esc(token)+"/export", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDeliveryReceipts fetches the counterparty-confirmed DELIVERY RECEIPTS
// behind a share token, plus a signed W3C credential of that record
// (CreddaDeliveryReceiptCredential, and CreddaAgentDeliveryCredential when the
// subject is an agent). GET /api/v1/verify/:token/delivery-receipts. No key
// required — the token is the capability, so an agent can present one string
// mid-negotiation and the counterparty verifies the credential offline.
//
// ConfirmedDeliveries counts ONLY outcomes a DISTINCT counterparty attested: an
// agent's own operator can never confirm its own work. This is a delivery
// record — not a safety, alignment or capability rating, and never a
// recommendation.
func (c *Client) GetDeliveryReceipts(ctx context.Context, token string) (*DeliveryReceipts, error) {
	var out DeliveryReceipts
	if err := c.get(ctx, "/verify/"+esc(token)+"/delivery-receipts", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RegisterAgent registers (or updates) an AGENT subject — a non-human scored
// subject. Writes no events and touches no score: an agent's record runs the
// identical deterministic formula as a person's. POST /api/v1/agents.
//
// By default the calling platform is declared as the agent's OPERATOR, which
// means events it reports for that agent are recorded but never counted as
// verified evidence — only a distinct counterparty can confirm a delivery. Set
// OperatedByReportingPlatform to a pointer-to-false (and name the operator) when
// reporting on someone else's agent. Requires events:write.
func (c *Client) RegisterAgent(ctx context.Context, input RegisterAgentInput) (*AgentSubject, error) {
	var out AgentSubject
	if err := c.post(ctx, "/agents", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAgent inspects an agent subject: its declared facts (claims, never
// evidence), its current deterministic score, and its delivery record split by
// whether a distinct counterparty confirmed each delivery.
// GET /api/v1/agents/:id. Requires scores:read.
func (c *Client) GetAgent(ctx context.Context, userID string) (*AgentDetail, error) {
	var out AgentDetail
	if err := c.get(ctx, "/agents/"+esc(userID), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetWebhookEvents fetches the outbound webhook event catalog: every event type
// the API can send, the delivery envelope, an example payload each, and
// signature-verification guidance. GET /api/v1/webhooks/events. No key required.
func (c *Client) GetWebhookEvents(ctx context.Context) (*WebhookEventCatalog, error) {
	var out WebhookEventCatalog
	if err := c.get(ctx, "/webhooks/events", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPlans fetches the developer plan catalog — the tiers, their scopes, rate
// limits and feature matrix. The same data the API enforces and the pricing
// page renders. GET /api/v1/plans. No API key required; no prices.
func (c *Client) GetPlans(ctx context.Context) (*PlanCatalog, error) {
	var out PlanCatalog
	if err := c.get(ctx, "/plans", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetErrorCatalog fetches the machine-readable error catalog: every stable
// code the API can return, with what it means, what to do about it, and
// whether a retry can help. Derived server-side from the same catalog the
// errors are built from, so it can never document a code that doesn't exist.
// GET /api/v1/errors. No API key required.
func (c *Client) GetErrorCatalog(ctx context.Context) (*ErrorCatalog, error) {
	var out ErrorCatalog
	if err := c.get(ctx, "/errors", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEnums fetches the self-describing wire enums — eventType, stakeLevel,
// scoreBand, disputeStatus and platformTier — each value described, with the
// facts that matter (stake weights, band floors, platform trust multipliers).
// Derived from the constants the API enforces, so a picker or validator built
// from it cannot drift. GET /api/v1/enums. No API key required.
func (c *Client) GetEnums(ctx context.Context) (*EnumCatalog, error) {
	var out EnumCatalog
	if err := c.get(ctx, "/enums", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetReasonCodes fetches the adverse-action reason-code catalog: the stable,
// versioned meaning of every reason code the scoring explanation can attribute,
// each with a consumer-facing description, a factor and a direction (adverse /
// supporting). Built for B2B2C partners that must issue an ECOA / Regulation B
// statement of specific reasons — read a subject's ranked codes from
// GET /score/explain (reasonCodes) and draw the notice from the adverse ones.
// GET /api/v1/reason-codes. No API key required. Credda supplies the
// attribution only — it is not a creditor and issues no notice.
func (c *Client) GetReasonCodes(ctx context.Context) (*ReasonCodeCatalog, error) {
	var out ReasonCodeCatalog
	if err := c.get(ctx, "/reason-codes", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDIDDocument fetches Credda's did:web DID document — issuer identity,
// verification keys, and service endpoints. GET /.well-known/did.json.
func (c *Client) GetDIDDocument(ctx context.Context) (*DIDDocument, error) {
	var out DIDDocument
	if err := c.getWellKnown(ctx, "/.well-known/did.json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTrustRegistry fetches the trust registry: Credda's own issuer entry plus
// any federated issuers it recognizes.
// GET /.well-known/credda-trust-registry.json.
func (c *Client) GetTrustRegistry(ctx context.Context) (*TrustRegistry, error) {
	var out TrustRegistry
	if err := c.getWellKnown(ctx, "/.well-known/credda-trust-registry.json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Platform reads (API key required) ───────────────────────────────────────

// GetScore returns the latest computed score for a user.
// GET /api/v1/users/:id/score.
func (c *Client) GetScore(ctx context.Context, userID string) (*ScorePayload, error) {
	var out ScorePayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/score", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScores is a batch score read: latest score + band for up to 100 users in
// one call. Unknown ids come back as entries with Error == "not_found" — a
// partial batch still succeeds, and results are in request order.
// POST /api/v1/users/scores.
func (c *Client) GetScores(ctx context.Context, userIDs []string) (*BatchScoresPayload, error) {
	var out BatchScoresPayload
	body := map[string]any{"userIds": userIDs}
	if err := c.post(ctx, "/users/scores", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScoreExplain returns a plain-language breakdown of a user's score.
// GET /api/v1/users/:id/score/explain.
func (c *Client) GetScoreExplain(ctx context.Context, userID string) (*ScoreExplainPayload, error) {
	var out ScoreExplainPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/score/explain", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScoreDelta returns a factor-level explanation of the user's last score
// change. Available is false until at least two computations exist.
// GET /api/v1/users/:id/score/delta.
func (c *Client) GetScoreDelta(ctx context.Context, userID string) (*ScoreDeltaPayload, error) {
	var out ScoreDeltaPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/score/delta", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScoreComponents returns the score reframed as named, independently
// 0–100-scored components. GET /api/v1/users/:id/score/components.
func (c *Client) GetScoreComponents(ctx context.Context, userID string) (*ScoreComponentsPayload, error) {
	var out ScoreComponentsPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/score/components", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Verified Earnings ───────────────────────────────────────────────────────

func earningsQuery(query *EarningsQuery) url.Values {
	qs := url.Values{}
	if query != nil {
		setInt(qs, "months", query.Months)
		setStr(qs, "from", query.From)
		setStr(qs, "to", query.To)
	}
	return qs
}

// GetEarnings returns a verified-earnings attestation: income ALREADY RECORDED
// on the ledger, bucketed by UTC month with a per-platform breakdown, plus
// stability metrics (median/mean monthly, volatility, months with earnings,
// longest consecutive run, trailing-12m total).
//
// Only counterparty/platform-VERIFIED outcomes are attested; unverified value is
// returned separately as UnverifiedReported and is never blended in. Amounts are
// platform-reported units — the ledger records no currency.
//
// This attests recorded outcomes. It is NOT an income verification for a credit
// decision, NOT a consumer report, and makes no representation of completeness.
// GET /api/v1/users/:id/earnings.
func (c *Client) GetEarnings(ctx context.Context, userID string, query *EarningsQuery) (*VerifiedEarnings, error) {
	var out VerifiedEarnings
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/earnings", earningsQuery(query)), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEarningsSummary returns the same attestation reduced to the figures a
// lender actually reads. It adds no fact the full breakdown does not hold.
// GET /api/v1/users/:id/earnings/summary.
func (c *Client) GetEarningsSummary(ctx context.Context, userID string, query *EarningsQuery) (*EarningsSummary, error) {
	var out EarningsSummary
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/earnings/summary", earningsQuery(query)), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MintEarningsCredential issues a signed Verified Earnings Credential (W3C
// VC-JWT, type CreddaEarningsCredential) so the subject can prove the recorded
// income offline. It carries a StatusList2021 credentialStatus like every Credda
// VC and refuses test-mode users.
// POST /api/v1/users/:id/earnings/credential.
func (c *Client) MintEarningsCredential(ctx context.Context, userID string, query *EarningsQuery, ttlSeconds *int) (*EarningsCredentialResult, error) {
	body := map[string]any{}
	if query != nil {
		if query.Months != nil {
			body["months"] = *query.Months
		}
		if query.From != "" {
			body["from"] = query.From
		}
		if query.To != "" {
			body["to"] = query.To
		}
	}
	if ttlSeconds != nil {
		body["ttlSeconds"] = *ttlSeconds
	}
	var out EarningsCredentialResult
	if err := c.post(ctx, "/users/"+esc(userID)+"/earnings/credential", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScoreHistory returns historical score snapshots, optionally windowed and
// cursor-paginated. Pass a nil query for defaults; use the returned NextCursor
// as query.Cursor to page. GET /api/v1/users/:id/score/history.
func (c *Client) GetScoreHistory(ctx context.Context, userID string, query *ScoreHistoryQuery) (*ScoreHistoryPayload, error) {
	qs := url.Values{}
	if query != nil {
		setStr(qs, "from", query.From)
		setStr(qs, "to", query.To)
		setInt(qs, "limit", query.Limit)
		setStr(qs, "cursor", query.Cursor)
	}
	var out ScoreHistoryPayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/score/history", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTimeline returns a unified, newest-first, cursor-paginated feed of events
// and score changes for a user. Read-only. GET /api/v1/users/:id/timeline.
func (c *Client) GetTimeline(ctx context.Context, userID string, query *TimelineQuery) (*TimelinePayload, error) {
	qs := url.Values{}
	if query != nil {
		setInt(qs, "limit", query.Limit)
		setStr(qs, "cursor", query.Cursor)
	}
	var out TimelinePayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/timeline", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPlatforms returns the platforms contributing to a user's score.
// GET /api/v1/users/:id/platforms.
func (c *Client) GetPlatforms(ctx context.Context, userID string) (*PlatformsPayload, error) {
	var out PlatformsPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/platforms", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRisk returns deterministic, ADVISORY anti-gaming risk signals for a user.
// These never affect the score. GET /api/v1/users/:id/risk.
func (c *Client) GetRisk(ctx context.Context, userID string) (*RiskPayload, error) {
	var out RiskPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/risk", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUsage returns this platform's own API consumption vs. its tier rate limit
// and monthly quota. Pass nil days for the API default (7, server max 400 —
// completed days beyond the live 90-day retention come from durable rollups).
// GET /api/v1/usage.
func (c *Client) GetUsage(ctx context.Context, days *int) (*UsagePayload, error) {
	qs := url.Values{}
	setInt(qs, "days", days)
	var out UsagePayload
	if err := c.get(ctx, withQuery("/usage", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUsageRange returns this platform's own API consumption for an explicit
// inclusive date range (ISO dates, YYYY-MM-DD) — for monthly statements.
// Mutually exclusive with the trailing-days window server-side. Completed days
// beyond the live 90-day counter retention are served from durable daily
// rollups; ranges are clamped to the server's history window (default 400
// days). CSV export (?format=csv) is a raw-HTTP use case; this returns parsed
// JSON. GET /api/v1/usage.
func (c *Client) GetUsageRange(ctx context.Context, from, to string) (*UsagePayload, error) {
	qs := url.Values{}
	qs.Set("from", from)
	qs.Set("to", to)
	var out UsagePayload
	if err := c.get(ctx, withQuery("/usage", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetActivity returns this platform's own activity log — the self-serve audit
// trail of what its keys and config did (events reported, webhooks/monitors
// changed, share tokens minted, keys issued). Strictly scoped to the calling
// platform's own rows, newest first, cursor-paginated. Uses the usage read
// scope (observability). GET /api/v1/activity.
func (c *Client) GetActivity(ctx context.Context, query *ActivityQuery) (*ActivityPayload, error) {
	qs := url.Values{}
	if query != nil {
		setInt(qs, "limit", query.Limit)
		setStr(qs, "cursor", query.Cursor)
		setStr(qs, "action", query.Action)
		setStr(qs, "from", query.From)
		setStr(qs, "to", query.To)
	}
	var out ActivityPayload
	if err := c.get(ctx, withQuery("/activity", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExportEvents returns the events this platform itself reported (data
// portability), oldest first (ledger order), cursor-paginated. Requires
// events:read (or coarse read). The CSV stream (?format=csv) is a raw-HTTP
// use case; this returns parsed JSON pages. GET /api/v1/events/export.
func (c *Client) ExportEvents(ctx context.Context, query *EventExportQuery) (*EventExportPayload, error) {
	qs := url.Values{}
	if query != nil {
		setInt(qs, "limit", query.Limit)
		setStr(qs, "cursor", query.Cursor)
		setStr(qs, "from", query.From)
		setStr(qs, "to", query.To)
	}
	var out EventExportPayload
	if err := c.get(ctx, withQuery("/events/export", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ProjectScore is a read-only what-if projection: what a user's score WOULD
// become if the given hypothetical events landed on the ledger. It never
// writes a snapshot or mutates the ledger.
// POST /api/v1/users/:id/score/project.
func (c *Client) ProjectScore(ctx context.Context, userID string, events []ProjectionEventInput) (*ScoreProjectionPayload, error) {
	var out ScoreProjectionPayload
	body := map[string]any{"events": events}
	if err := c.post(ctx, "/users/"+esc(userID)+"/score/project", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Writes (API key required) ───────────────────────────────────────────────

// ReportEvent ingests an outcome event into the append-only ledger. Pass a
// stable idempotencyKey (empty string to omit) to make retries exactly-once —
// strongly recommended. POST /api/v1/events.
func (c *Client) ReportEvent(ctx context.Context, input ReportEventInput, idempotencyKey string) (*ReportEventResult, error) {
	var headers map[string]string
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	var out ReportEventResult
	if err := c.post(ctx, "/events", input, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReportEvents reports up to 100 events in one call. Partial success: the
// result lists each item's outcome. Give an item an IdempotencyKey so a
// retried batch is exactly-once. POST /api/v1/events/batch.
func (c *Client) ReportEvents(ctx context.Context, events []BatchEventInput) (*BatchEventsResult, error) {
	var out BatchEventsResult
	body := map[string]any{"events": events}
	if err := c.post(ctx, "/events/batch", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MintShareToken mints (or rotates) a public share token for a user — the
// capability behind trust badges, the verify page, and the portable export.
// POST /api/v1/users/:id/share-token.
func (c *Client) MintShareToken(ctx context.Context, userID string) (*ShareTokenResult, error) {
	var out ShareTokenResult
	if err := c.post(ctx, "/users/"+esc(userID)+"/share-token", map[string]any{}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeShareToken revokes a user's share token, immediately invalidating
// every embed using it. DELETE /api/v1/users/:id/share-token.
func (c *Client) RevokeShareToken(ctx context.Context, userID string) error {
	return c.delete(ctx, "/users/"+esc(userID)+"/share-token")
}

// ResolveDispute resolves a dispute the calling platform owns. DisputeForUser
// clears it (severity 0); DisputeAgainstUser upholds it. Triggers a score
// recompute and a dispute.resolved webhook.
// PATCH /api/v1/disputes/:id/resolve.
func (c *Client) ResolveDispute(ctx context.Context, disputeID, outcome string) (*DisputeResult, error) {
	var out DisputeResult
	body := map[string]any{"outcome": outcome}
	if err := c.patch(ctx, "/disputes/"+esc(disputeID)+"/resolve", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Webhook management (API key required) ───────────────────────────────────
//
// To VERIFY received webhooks, use VerifyWebhookSignature /
// ConstructWebhookEvent instead — those need no client and no network.

// CreateWebhook subscribes an HTTPS endpoint to trust events. The signing
// secret is returned ONCE — store it to verify deliveries.
// POST /api/v1/webhooks.
func (c *Client) CreateWebhook(ctx context.Context, input CreateWebhookInput) (*CreateWebhookResult, error) {
	var out CreateWebhookResult
	if err := c.post(ctx, "/webhooks", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListWebhooks lists this platform's webhooks (secrets are never returned).
// GET /api/v1/webhooks.
func (c *Client) ListWebhooks(ctx context.Context) (*WebhookListResult, error) {
	var out WebhookListResult
	if err := c.get(ctx, "/webhooks", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWebhook patches a webhook (url / events / description / isActive).
// Re-enabling resets its failure health. PATCH /api/v1/webhooks/:id.
func (c *Client) UpdateWebhook(ctx context.Context, id string, patch UpdateWebhookInput) (*WebhookUpdateResult, error) {
	var out WebhookUpdateResult
	if err := c.patch(ctx, "/webhooks/"+esc(id), patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhook deletes a webhook subscription. DELETE /api/v1/webhooks/:id.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.delete(ctx, "/webhooks/"+esc(id))
}

// TestWebhook sends a synthetic signed delivery to confirm connectivity and
// signature verification. POST /api/v1/webhooks/:id/test.
func (c *Client) TestWebhook(ctx context.Context, id string) (*WebhookTestResult, error) {
	var out WebhookTestResult
	if err := c.post(ctx, "/webhooks/"+esc(id)+"/test", map[string]any{}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetWebhookDeliveries returns recent delivery attempts for a webhook
// (debugging), cursor-paginated. Pass nil limit / empty cursor to omit them.
// GET /api/v1/webhooks/:id/deliveries.
func (c *Client) GetWebhookDeliveries(ctx context.Context, id string, limit *int, cursor string) (*WebhookDeliveriesResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out WebhookDeliveriesResult
	if err := c.get(ctx, withQuery("/webhooks/"+esc(id)+"/deliveries", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRecentWebhookEvents returns recent outbound events across ALL of your
// webhook endpoints — the "perform-list" sample-data source automation
// platforms (Zapier, Make, n8n) need to show a trigger's output BEFORE any
// event has fired. Each item is the delivery envelope exactly as sent, so a
// field mapping built against a sample keeps working on real deliveries.
//
// With no retained deliveries yet, representative payloads from the event
// catalog are returned instead, flagged IsExample with Source == "examples" —
// never present those as something that actually happened.
//
// Pass nil limit / empty cursor to omit them; eventTypes filters to specific
// event types (an unknown type is a 400, not an empty result).
// GET /api/v1/webhooks/deliveries.
func (c *Client) GetRecentWebhookEvents(ctx context.Context, limit *int, cursor string, eventTypes ...string) (*RecentWebhookEventsResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	if len(eventTypes) > 0 {
		setStr(qs, "eventType", strings.Join(eventTypes, ","))
	}
	var out RecentWebhookEventsResult
	if err := c.get(ctx, withQuery("/webhooks/deliveries", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Score monitors (continuous monitoring, API key required) ────────────────
//
// Edge-triggered threshold/band watches that deliver monitor.triggered events
// through your subscribed webhooks. Notification config only — a monitor never
// affects a score. Uses the `webhooks` scope.

// CreateMonitor registers a monitor on one of your users. At least one
// condition is required: BelowScore (downward crossing — also fires on a FIRST
// score already below the threshold), AboveScore (upward crossing only), or
// OnBandChange. POST /api/v1/monitors.
func (c *Client) CreateMonitor(ctx context.Context, input CreateMonitorInput) (*MonitorResult, error) {
	var out MonitorResult
	if err := c.post(ctx, "/monitors", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListMonitors lists this platform's monitors, cursor-paginated. Pass nil
// limit / empty cursor to omit them. GET /api/v1/monitors.
func (c *Client) ListMonitors(ctx context.Context, limit *int, cursor string) (*MonitorListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out MonitorListResult
	if err := c.get(ctx, withQuery("/monitors", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMonitor fetches one monitor. GET /api/v1/monitors/:id.
func (c *Client) GetMonitor(ctx context.Context, id string) (*MonitorResult, error) {
	var out MonitorResult
	if err := c.get(ctx, "/monitors/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMonitor patches a monitor's thresholds / OnBandChange / IsActive. The
// updated monitor must keep at least one condition.
// PATCH /api/v1/monitors/:id.
func (c *Client) UpdateMonitor(ctx context.Context, id string, patch UpdateMonitorInput) (*MonitorResult, error) {
	var out MonitorResult
	if err := c.patch(ctx, "/monitors/"+esc(id), patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMonitor deletes a monitor (hard delete — it is config, not ledger
// data). DELETE /api/v1/monitors/:id.
func (c *Client) DeleteMonitor(ctx context.Context, id string) error {
	return c.delete(ctx, "/monitors/"+esc(id))
}

// ── Bulk screenings (async batch score reads, API key required) ─────────────
//
// Roster-scale batch reads: up to 10,000 ids per job (vs. GetScores' 100).
// STRICTLY READ-ONLY — a screening never writes events, snapshots, or
// anything score-side. Uses the `scores` scope, same as GetScores.

// CreateScreening submits an async bulk screening. Ids are deduped
// server-side; each resolves through the same lookup as GetScores. Jobs of at
// most 100 deduped ids are processed inline — the returned job is usually
// already COMPLETED; larger jobs are queued: poll GetScreening until
// COMPLETED, then fetch GetScreeningResults. POST /api/v1/screenings.
func (c *Client) CreateScreening(ctx context.Context, userIDs []string) (*ScreeningResult, error) {
	var out ScreeningResult
	body := map[string]any{"userIds": userIDs}
	if err := c.post(ctx, "/screenings", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListScreenings lists this platform's screening jobs (status + summary
// only), cursor-paginated. Pass nil limit / empty cursor to omit them.
// GET /api/v1/screenings.
func (c *Client) ListScreenings(ctx context.Context, limit *int, cursor string) (*ScreeningListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out ScreeningListResult
	if err := c.get(ctx, withQuery("/screenings", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScreening fetches one screening job's status + summary (no results
// payload). GET /api/v1/screenings/:id.
func (c *Client) GetScreening(ctx context.Context, id string) (*ScreeningResult, error) {
	var out ScreeningResult
	if err := c.get(ctx, "/screenings/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScreeningResults fetches the full per-user results of a COMPLETED
// screening. Returns a 409 APIError (SCREENING_NOT_COMPLETED) while the job
// is still queued/running or if it failed. CSV export (?format=csv) is a
// raw-HTTP use case — this returns parsed JSON.
// GET /api/v1/screenings/:id/results.
func (c *Client) GetScreeningResults(ctx context.Context, id string) (*ScreeningResultsResult, error) {
	var out ScreeningResultsResult
	if err := c.get(ctx, "/screenings/"+esc(id)+"/results", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Data ingress: field-mapping ingest + historical CSV import ─────────────
//
// Send YOUR payload shape (no client-side transformation) or backfill a CSV.
// Both write through the SAME append-only path as ReportEvent — idempotency,
// velocity guard, audit trail, asynchronous score recomputation — and neither
// contains any scoring logic. Uses the `events` scope, same as ReportEvent.
//
// A mapping is DECLARATIVE DATA, never code (see IngestMapping).
//
// isVerified defaults to false. It is only honoured for a record whose mapping
// also resolves verifiedBy (the third party who witnessed the outcome);
// otherwise the record still ingests, downgraded, with a warning.

// Ingest reports records in your own shape via a field mapping. Up to 100
// records per call; partial success — a bad record fails individually with its
// index and reason. Pass either mapping (inline) or mappingID (stored), not
// both; leave the other zero. POST /api/v1/ingest.
func (c *Client) Ingest(ctx context.Context, records []any, mapping IngestMapping, mappingID string) (*IngestResult, error) {
	body := map[string]any{"records": records}
	if mapping != nil {
		body["mapping"] = mapping
	}
	if mappingID != "" {
		body["mappingId"] = mappingID
	}
	var out IngestResult
	if err := c.post(ctx, "/ingest", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateMapping saves a reusable named mapping. Declare your translation once,
// then call Ingest with its id forever. POST /api/v1/ingest/mappings.
func (c *Client) CreateMapping(ctx context.Context, name, description string, mapping IngestMapping) (*MappingResult, error) {
	body := map[string]any{"name": name, "mapping": mapping}
	if description != "" {
		body["description"] = description
	}
	var out MappingResult
	if err := c.post(ctx, "/ingest/mappings", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListMappings lists your stored mappings, cursor-paginated. Pass nil limit /
// empty cursor to omit them. GET /api/v1/ingest/mappings.
func (c *Client) ListMappings(ctx context.Context, limit *int, cursor string) (*MappingListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out MappingListResult
	if err := c.get(ctx, withQuery("/ingest/mappings", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMapping fetches one stored mapping. GET /api/v1/ingest/mappings/:id.
func (c *Client) GetMapping(ctx context.Context, id string) (*MappingResult, error) {
	var out MappingResult
	if err := c.get(ctx, "/ingest/mappings/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMapping deletes a stored mapping. Config, not ledger data — events
// already ingested through it are untouched.
// DELETE /api/v1/ingest/mappings/:id.
func (c *Client) DeleteMapping(ctx context.Context, id string) error {
	return c.delete(ctx, "/ingest/mappings/"+esc(id))
}

// CreateImport backfills historical outcomes from a CSV. Mapping paths are
// COLUMN NAMES. Files of at most 100 rows are processed inline (the returned
// job usually already reads COMPLETED); larger files are queued — poll
// GetImport, then GetImportErrors to fix and re-upload (idempotency keys make
// that safe). Imported events keep their REAL dates, so scores recompute over
// true history. POST /api/v1/imports.
func (c *Client) CreateImport(ctx context.Context, csv string, mapping IngestMapping, mappingID string) (*ImportResult, error) {
	body := map[string]any{"csv": csv}
	if mapping != nil {
		body["mapping"] = mapping
	}
	if mappingID != "" {
		body["mappingId"] = mappingID
	}
	var out ImportResult
	if err := c.post(ctx, "/imports", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListImports lists your CSV imports, cursor-paginated. GET /api/v1/imports.
func (c *Client) ListImports(ctx context.Context, limit *int, cursor string) (*ImportListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out ImportListResult
	if err := c.get(ctx, withQuery("/imports", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetImport fetches one import's status + counts. GET /api/v1/imports/:id.
func (c *Client) GetImport(ctx context.Context, id string) (*ImportResult, error) {
	var out ImportResult
	if err := c.get(ctx, "/imports/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetImportErrors returns every rejected row with its 1-based data-row number
// and the exact reason, plus non-fatal warnings such as an isVerified
// downgrade — so a corrected file can be re-uploaded safely.
// GET /api/v1/imports/:id/errors.
func (c *Client) GetImportErrors(ctx context.Context, id string, limit, offset *int) (*ImportErrorsResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setInt(qs, "offset", offset)
	var out ImportErrorsResult
	if err := c.get(ctx, withQuery("/imports/"+esc(id)+"/errors", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
