package credda

import (
	"context"
	"net/url"
)

// This file brings the Go client up to parity with the TypeScript SDK
// (packages/sdk/src/lib/client.ts) and the routes mounted in
// packages/api/src/app.ts: benchmarks, the book query/export, trust summaries,
// wallet issuance, webhook replay, confirmation requests, threshold policies and
// Open Badges. Everything here is additive.

// ─── Types: book query / export (GET /api/v1/users) ─────────────────────────

// SubjectSummary is one subject in your book — operational fields only.
//
// FinalScore and ScoreBand are POINTERS because a subject can be in your book
// (you have reported events for them) while the engine has not yet produced a
// score: those rows report null, never a placeholder number. Filter them in or
// out with BookFilterQuery.HasScore.
type SubjectSummary struct {
	ExternalID         string   `json:"externalId"`
	SubjectType        string   `json:"subjectType"`
	FinalScore         *float64 `json:"finalScore"`
	ScoreBand          *string  `json:"scoreBand"`
	ScoreFrozen        bool     `json:"scoreFrozen"`
	VerificationDepth  *float64 `json:"verificationDepth"`
	EventCount         int      `json:"eventCount"`
	VerifiedEventCount int      `json:"verifiedEventCount"`
	LastActivityAt     *string  `json:"lastActivityAt"`
	RegisteredAt       string   `json:"registeredAt"`
	ComputedAt         *string  `json:"computedAt"`
}

// ListUsersPayload is a cursor-paginated page of your subjects.
type ListUsersPayload struct {
	Data       []SubjectSummary `json:"data"`
	Count      int              `json:"count"`
	NextCursor *string          `json:"nextCursor"`
}

// BookFilterQuery is the CLOSED filter vocabulary shared by ListUsers and
// GetBookSummary. Nil/empty fields are omitted. SubjectType is "PERSON",
// "AGENT" or "ORGANIZATION".
//
// It is closed on purpose: there is deliberately no free-form query language,
// because the fixed set is what lets the server guarantee tenant scoping for
// every combination of filters.
//
// HasScore=false selects exactly the subjects still awaiting a first score, and
// cannot be combined with ScoreMin/ScoreMax/Band (an unscored subject has no
// score to compare) — the server refuses that pair with a 400 rather than
// returning a confidently empty page.
type BookFilterQuery struct {
	ScoreMin          *float64
	ScoreMax          *float64
	Band              string
	HasScore          *bool
	ScoreFrozen       *bool
	SubjectType       string
	ActiveSince       string
	RegisteredSince   string
	RegisteredBefore  string
	HasVerifiedEvents *bool
	MinVerifiedEvents *int
}

// ListUsersQuery is the closed filter set plus paging for ListUsers. Sort is one
// of "score" / "lastActivity" / "registered" / "externalId"; Order is "asc" /
// "desc".
type ListUsersQuery struct {
	BookFilterQuery
	Sort   string
	Order  string
	Limit  *int
	Cursor string
}

// BookSummaryBand is one band bucket in a segment summary. Share is a
// percentage of the SCORED population, nil when that population is empty.
type BookSummaryBand struct {
	Band     string   `json:"band"`
	MinScore float64  `json:"minScore"`
	Count    int      `json:"count"`
	Share    *float64 `json:"share"`
}

// BookSummaryCentral is the central tendency over a segment's SCORED subjects.
// Both members are nil when nothing in the segment is scored — a 0 there would
// read as a real, catastrophic score rather than "nothing has been scored yet".
type BookSummaryCentral struct {
	Median *float64 `json:"median"`
	Mean   *float64 `json:"mean"`
}

// BookSummaryAggregationSkipped states why the distribution was not computed.
type BookSummaryAggregationSkipped struct {
	Reason      string `json:"reason"`
	MaxSubjects int    `json:"maxSubjects"`
}

// BookSummaryPayload is the counts + score shape for a segment of your book.
// Matched is always exact; the aggregates are nil only when the population
// exceeded the server's fold cap, in which case AggregationSkipped says so — a
// partial aggregate is never presented as a whole-segment one.
type BookSummaryPayload struct {
	FormulaVersion     string                         `json:"formulaVersion"`
	Matched            int                            `json:"matched"`
	Scored             *int                           `json:"scored"`
	Unscored           *int                           `json:"unscored"`
	Central            *BookSummaryCentral            `json:"central"`
	BandDistribution   []BookSummaryBand              `json:"bandDistribution"`
	AggregationSkipped *BookSummaryAggregationSkipped `json:"aggregationSkipped,omitempty"`
}

// ─── Types: trust summary ────────────────────────────────────────────────────

// TrustSummaryEvidence is the recorded evidence a trust summary rests on.
type TrustSummaryEvidence struct {
	FinalScore        float64 `json:"finalScore"`
	ScoreBand         string  `json:"scoreBand"`
	ConfidenceLevel   string  `json:"confidenceLevel"`
	CompletionRate    float64 `json:"completionRate"`
	OnTimeRate        float64 `json:"onTimeRate"`
	VerifiedEvents    int     `json:"verifiedEvents"`
	TotalEvents       int     `json:"totalEvents"`
	DistinctPlatforms int     `json:"distinctPlatforms"`
}

// TrustSummaryAI is the optional advisory AI narrative (inert unless enabled).
type TrustSummaryAI struct {
	Enabled   bool    `json:"enabled"`
	Narrative *string `json:"narrative"`
}

// TrustSummaryPayload is a deterministic, evidence-based trust summary. It
// explains; it never decides — there is no verdict endpoint by design.
type TrustSummaryPayload struct {
	UserID         string                `json:"userId"`
	Available      bool                  `json:"available"`
	Summary        string                `json:"summary"`
	Strengths      []string              `json:"strengths,omitempty"`
	Risks          []string              `json:"risks,omitempty"`
	Evidence       *TrustSummaryEvidence `json:"evidence,omitempty"`
	Advisory       string                `json:"advisory,omitempty"`
	FormulaVersion string                `json:"formulaVersion,omitempty"`
	ComputedAt     string                `json:"computedAt,omitempty"`
	AI             *TrustSummaryAI       `json:"ai,omitempty"`
}

// ─── Types: wallet issuance (OID4VCI) ───────────────────────────────────────

// CredentialIssuerMetadata is the OID4VCI issuer discovery document.
type CredentialIssuerMetadata struct {
	CredentialIssuer                  string                    `json:"credential_issuer"`
	CredentialEndpoint                string                    `json:"credential_endpoint"`
	NonceEndpoint                     string                    `json:"nonce_endpoint,omitempty"`
	AuthorizationServers              []string                  `json:"authorization_servers,omitempty"`
	CredentialConfigurationsSupported map[string]map[string]any `json:"credential_configurations_supported"`
	Display                           []map[string]any          `json:"display,omitempty"`
}

// CredentialOffer is the pre-authorized offer inside a CredentialOfferResult.
type CredentialOffer struct {
	CredentialIssuer           string                    `json:"credential_issuer"`
	CredentialConfigurationIDs []string                  `json:"credential_configuration_ids"`
	Grants                     map[string]map[string]any `json:"grants"`
}

// CredentialOfferResult is the result of CreateCredentialOffer.
type CredentialOfferResult struct {
	CredentialOffer    CredentialOffer `json:"credentialOffer"`
	CredentialOfferURI string          `json:"credentialOfferUri"`
	ExpiresIn          int             `json:"expiresIn"`
	Scope              string          `json:"scope"`
	CredentialIssuer   string          `json:"credentialIssuer"`
	IssuerMetadata     string          `json:"issuerMetadata"`
}

// CredentialOfferInput are the options for CreateCredentialOffer. Nil/empty
// fields fall back to the issuer default.
type CredentialOfferInput struct {
	CredentialConfigurationIDs []string `json:"credentialConfigurationIds,omitempty"`
	Scope                      string   `json:"scope,omitempty"`
}

// ─── Types: webhook replay ──────────────────────────────────────────────────

// WebhookReplayResult is the result of ReplayWebhookDelivery.
type WebhookReplayResult struct {
	Status     string  `json:"status"`
	Success    bool    `json:"success"`
	StatusCode *int    `json:"statusCode"`
	Error      *string `json:"error"`
}

// ─── Types: benchmarks ──────────────────────────────────────────────────────

// BenchmarkStatistics are aggregate order statistics over a cohort.
type BenchmarkStatistics struct {
	Median float64 `json:"median"`
	Mean   float64 `json:"mean"`
	P25    float64 `json:"p25"`
	P75    float64 `json:"p75"`
	P90    float64 `json:"p90"`
}

// BenchmarkBandCount is one band's count in a cohort's histogram.
type BenchmarkBandCount struct {
	Band     string  `json:"band"`
	MinScore float64 `json:"minScore"`
	Count    int     `json:"count"`
}

// BenchmarkCohort is one cohort's distribution. When Available is false (below
// the k-anonymity floor) only Dimension / Cohort / Reason / MinimumCohortSize
// are populated and the numeric fields are zero.
type BenchmarkCohort struct {
	Available         bool                 `json:"available"`
	Dimension         string               `json:"dimension"`
	Cohort            string               `json:"cohort"`
	CohortSize        int                  `json:"cohortSize,omitempty"`
	Statistics        *BenchmarkStatistics `json:"statistics,omitempty"`
	BandDistribution  []BenchmarkBandCount `json:"bandDistribution,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	MinimumCohortSize int                  `json:"minimumCohortSize,omitempty"`
}

// BenchmarkDistributionPayload is GET /api/v1/benchmarks/distribution. With a
// cohort set, the top level itself carries the single cohort's fields (the
// BenchmarkCohort fields are populated). Without one, the Dimension /
// PopulationSize / Cohorts fields describe the whole dimension.
type BenchmarkDistributionPayload struct {
	BenchmarkVersion string `json:"benchmarkVersion"`
	FormulaVersion   string `json:"formulaVersion"`
	// Whole-dimension form:
	Dimension      string            `json:"dimension,omitempty"`
	PopulationSize int               `json:"populationSize,omitempty"`
	Cohorts        []BenchmarkCohort `json:"cohorts,omitempty"`
	// Single-cohort form (spread from the cohort):
	Available         bool                 `json:"available,omitempty"`
	Cohort            string               `json:"cohort,omitempty"`
	CohortSize        int                  `json:"cohortSize,omitempty"`
	Statistics        *BenchmarkStatistics `json:"statistics,omitempty"`
	BandDistribution  []BenchmarkBandCount `json:"bandDistribution,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	MinimumCohortSize int                  `json:"minimumCohortSize,omitempty"`
}

// UserBenchmarkPayload is GET /api/v1/users/:id/benchmark — where the subject
// sits within its cohort. When Available is false, Reason is "insufficient_data"
// (cohort below the floor) or "no_score" (subject not scored yet). A percentile
// is not a verdict.
type UserBenchmarkPayload struct {
	UserID            string               `json:"userId"`
	Available         bool                 `json:"available"`
	Dimension         string               `json:"dimension,omitempty"`
	Cohort            string               `json:"cohort,omitempty"`
	CohortSize        int                  `json:"cohortSize,omitempty"`
	FinalScore        float64              `json:"finalScore,omitempty"`
	Percentile        float64              `json:"percentile,omitempty"`
	Comparison        string               `json:"comparison,omitempty"`
	Distribution      *BenchmarkStatistics `json:"distribution,omitempty"`
	BandDistribution  []BenchmarkBandCount `json:"bandDistribution,omitempty"`
	FormulaVersion    string               `json:"formulaVersion,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	MinimumCohortSize int                  `json:"minimumCohortSize,omitempty"`
}

// BenchmarkDimensionValue is one allowed cohort value on a dimension.
type BenchmarkDimensionValue struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

// BenchmarkDimension documents one cohort dimension.
type BenchmarkDimension struct {
	Dimension     string                    `json:"dimension"`
	Description   string                    `json:"description"`
	Justification string                    `json:"justification"`
	Values        []BenchmarkDimensionValue `json:"values"`
}

// BenchmarkCatalog is the public benchmark catalog (GET /api/v1/benchmarks).
type BenchmarkCatalog struct {
	BenchmarkVersion string `json:"benchmarkVersion"`
	FormulaVersion   string `json:"formulaVersion"`
	Note             string `json:"note"`
	KAnonymity       struct {
		MinimumCohortSize int    `json:"minimumCohortSize"`
		Guarantee         string `json:"guarantee"`
	} `json:"kAnonymity"`
	Dimensions        []BenchmarkDimension `json:"dimensions"`
	Statistics        []string             `json:"statistics"`
	SubjectComparison struct {
		Description  string   `json:"description"`
		CoarseLabels []string `json:"coarseLabels"`
	} `json:"subjectComparison"`
	Disclosures   []string `json:"disclosures"`
	Deterministic bool     `json:"deterministic"`
}

// ─── Types: confirmation requests ────────────────────────────────────────────

// ConfirmationRequest is a confirmation request (the object under the
// "confirmation" key of every confirmation route).
type ConfirmationRequest struct {
	ID                string   `json:"id"`
	SubjectExternalID string   `json:"subjectExternalId"`
	EventType         string   `json:"eventType"`
	StakeLevel        string   `json:"stakeLevel"`
	TransactionValue  *float64 `json:"transactionValue"`
	DueDate           *string  `json:"dueDate"`
	CompletedAt       *string  `json:"completedAt"`
	CounterpartyRef   string   `json:"counterpartyRef"`
	CounterpartyName  *string  `json:"counterpartyName"`
	Description       *string  `json:"description"`
	// ReturnURL is the post-decision redirect configured for the HOSTED page
	// (nil when unset). It is the requesting platform's own configuration and is
	// never shown to the counterparty — the hosted page reads it server-side.
	ReturnURL        *string `json:"returnUrl"`
	Status           string  `json:"status"`
	ExpiresAt        string  `json:"expiresAt"`
	ResultingEventID *string `json:"resultingEventId"`
	DecidedAt        *string `json:"decidedAt"`
	CreatedAt        string  `json:"createdAt"`
}

// CreateConfirmationInput is the body for CreateConfirmationRequest. UserID,
// EventType and CounterpartyRef are required; the rest are optional. EventType
// must be a confirmable (ingestable) event type. ExpiresInDays is 1–90.
type CreateConfirmationInput struct {
	UserID           string         `json:"userId"`
	EventType        string         `json:"eventType"`
	StakeLevel       string         `json:"stakeLevel,omitempty"`
	TransactionValue *float64       `json:"transactionValue,omitempty"`
	DueDate          string         `json:"dueDate,omitempty"`
	CompletedAt      string         `json:"completedAt,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CounterpartyRef  string         `json:"counterpartyRef"`
	CounterpartyName string         `json:"counterpartyName,omitempty"`
	Description      string         `json:"description,omitempty"`
	// ReturnURL sends the counterparty back to your own page after they decide
	// on the HOSTED page. Must be an absolute https URL on a public host with no
	// embedded credentials — loopback/private/link-local destinations are
	// refused (400). The redirect carries `credda_confirmation=confirmed|declined`
	// and NOTHING else: never the token, the subject id, the counterparty ref,
	// or a score.
	ReturnURL     string `json:"returnUrl,omitempty"`
	ExpiresInDays *int   `json:"expiresInDays,omitempty"`
}

// ConfirmationCreateResult is the result of CreateConfirmationRequest. The token
// is shown ONCE — deliver it to the counterparty over your own channel.
type ConfirmationCreateResult struct {
	Confirmation      ConfirmationRequest `json:"confirmation"`
	ConfirmationToken string              `json:"confirmationToken"`
	// ConfirmURL is Credda's HOSTED confirmation page — the zero-frontend path.
	// It is server-rendered, ships no JavaScript and needs no account. Credda
	// never delivers this link: you do, over your own channel.
	ConfirmURL string `json:"confirmUrl"`
	// PreviewURL / RespondURL are for platforms building their own UI instead.
	PreviewURL string `json:"previewUrl"`
	RespondURL string `json:"respondUrl"`
}

// ConfirmationResult is the envelope returned by GetConfirmation / CancelConfirmation.
type ConfirmationResult struct {
	Confirmation ConfirmationRequest `json:"confirmation"`
}

// ConfirmationBatchItemResult is one entry in a ConfirmationBatchResult —
// partial success. An ok item carries its one-time token + hosted confirmUrl; a
// failed one carries the reason + code (e.g. CONFIRMATION_SELF). No item writes
// anything to the ledger.
type ConfirmationBatchItemResult struct {
	Index  int    `json:"index"`
	OK     bool   `json:"ok"`
	UserID string `json:"userId"`
	// ID is the created request id (ok items only).
	ID string `json:"id,omitempty"`
	// Status is PENDING (ok items only).
	Status string `json:"status,omitempty"`
	// ConfirmationToken is the one-time token — shown ONCE, deliver it to the
	// counterparty (ok items only).
	ConfirmationToken string `json:"confirmationToken,omitempty"`
	// ConfirmURL is the hosted "Confirm with Credda" page for this request (ok
	// items only).
	ConfirmURL string `json:"confirmUrl,omitempty"`
	// Error is the human-readable reason (failed items only).
	Error string `json:"error,omitempty"`
	// Code is the error code, e.g. CONFIRMATION_SELF (failed items only).
	Code string `json:"code,omitempty"`
}

// ConfirmationBatchResult is the result of CreateConfirmationBatch — partial
// success, one entry per input request. Nothing is written to the ledger by any
// item until its named counterparty confirms.
type ConfirmationBatchResult struct {
	Total   int                           `json:"total"`
	Created int                           `json:"created"`
	Failed  int                           `json:"failed"`
	Results []ConfirmationBatchItemResult `json:"results"`
}

// ConfirmationListResult is the cursor-paginated envelope from ListConfirmations.
type ConfirmationListResult struct {
	Data       []ConfirmationRequest `json:"data"`
	NextCursor *string               `json:"nextCursor"`
}

// ConfirmationPreview is the PII-free subset a counterparty sees.
type ConfirmationPreview struct {
	ID               string   `json:"id"`
	Platform         string   `json:"platform"`
	Status           string   `json:"status"`
	EventType        string   `json:"eventType"`
	StakeLevel       string   `json:"stakeLevel"`
	TransactionValue *float64 `json:"transactionValue"`
	DueDate          *string  `json:"dueDate"`
	CompletedAt      *string  `json:"completedAt"`
	CounterpartyName *string  `json:"counterpartyName"`
	Description      *string  `json:"description"`
	ExpiresAt        string   `json:"expiresAt"`
}

// ConfirmationPreviewResult is the envelope from PreviewConfirmation.
type ConfirmationPreviewResult struct {
	Confirmation ConfirmationPreview `json:"confirmation"`
}

// ConfirmationRespondResult is the result of RespondToConfirmation. On "confirm"
// the proposed event is written (verified) and EventID is set; on "decline"
// nothing is written.
type ConfirmationRespondResult struct {
	Status       string              `json:"status"`
	Confirmation ConfirmationRequest `json:"confirmation"`
	EventID      *string             `json:"eventId"`
}

// ─── Types: reference / employment-verification requests ─────────────────────

// ReferenceRequest is a reference request (the object under the "reference" key
// of every reference route). Label/Issuer/Jurisdiction/Reference are
// display-only and are never scored or ranked.
type ReferenceRequest struct {
	ID                string  `json:"id"`
	SubjectExternalID string  `json:"subjectExternalId"`
	Category          string  `json:"category"`
	Label             *string `json:"label"`
	Issuer            *string `json:"issuer"`
	Jurisdiction      *string `json:"jurisdiction"`
	Reference         *string `json:"reference"`
	CounterpartyRef   string  `json:"counterpartyRef"`
	CounterpartyName  *string `json:"counterpartyName"`
	Description       *string `json:"description"`
	// ReturnURL is the post-decision redirect configured for the HOSTED page
	// (nil when unset). It is the requesting platform's own configuration and is
	// never shown to the counterparty — the hosted page reads it server-side.
	ReturnURL        *string `json:"returnUrl"`
	Status           string  `json:"status"`
	ExpiresAt        string  `json:"expiresAt"`
	ResultingEventID *string `json:"resultingEventId"`
	DecidedAt        *string `json:"decidedAt"`
	CreatedAt        string  `json:"createdAt"`
}

// CreateReferenceInput is the body for CreateReferenceRequest. UserID, Category
// and CounterpartyRef are required; the rest are optional. Category is one of
// "employment" / "education" / "certification" / "skill". CounterpartyRef must
// not name the subject (a person cannot be their own reference → 400
// REFERENCE_SELF). ExpiresInDays is 1–90.
type CreateReferenceInput struct {
	UserID           string `json:"userId"`
	Category         string `json:"category"`
	Label            string `json:"label,omitempty"`
	Issuer           string `json:"issuer,omitempty"`
	Jurisdiction     string `json:"jurisdiction,omitempty"`
	Reference        string `json:"reference,omitempty"`
	CounterpartyRef  string `json:"counterpartyRef"`
	CounterpartyName string `json:"counterpartyName,omitempty"`
	Description      string `json:"description,omitempty"`
	// ReturnURL sends the reference back to your own page after they decide on
	// the HOSTED page. Must be an absolute https URL on a public host with no
	// embedded credentials — loopback/private/link-local destinations are
	// refused (400).
	ReturnURL     string `json:"returnUrl,omitempty"`
	ExpiresInDays *int   `json:"expiresInDays,omitempty"`
}

// ReferenceCreateResult is the result of CreateReferenceRequest. The token is
// shown ONCE — deliver it to the named reference over your own channel.
type ReferenceCreateResult struct {
	Reference      ReferenceRequest `json:"reference"`
	ReferenceToken string           `json:"referenceToken"`
	// ReferenceURL is Credda's HOSTED reference page — the zero-frontend path.
	// Credda never delivers this link: you do, over your own channel.
	ReferenceURL string `json:"referenceUrl"`
	// PreviewURL / RespondURL are for platforms building their own UI instead.
	PreviewURL string `json:"previewUrl"`
	RespondURL string `json:"respondUrl"`
}

// ReferenceResult is the envelope returned by GetReference / CancelReference.
type ReferenceResult struct {
	Reference ReferenceRequest `json:"reference"`
}

// ReferenceListResult is the cursor-paginated envelope from ListReferences.
type ReferenceListResult struct {
	Data       []ReferenceRequest `json:"data"`
	NextCursor *string            `json:"nextCursor"`
}

// ReferencePreview is the PII-free subset a counterparty sees. It never carries
// the raw subject id or the counterpartyRef matching key.
type ReferencePreview struct {
	ID               string  `json:"id"`
	Platform         string  `json:"platform"`
	Status           string  `json:"status"`
	Category         string  `json:"category"`
	Label            *string `json:"label"`
	Issuer           *string `json:"issuer"`
	Jurisdiction     *string `json:"jurisdiction"`
	Reference        *string `json:"reference"`
	CounterpartyName *string `json:"counterpartyName"`
	Description      *string `json:"description"`
	ExpiresAt        string  `json:"expiresAt"`
}

// ReferencePreviewResult is the envelope from PreviewReference.
type ReferencePreviewResult struct {
	Reference ReferencePreview `json:"reference"`
}

// ReferenceRespondResult is the result of RespondToReference. On "confirm" the
// qualification is recorded (verified) and EventID is set; on "decline" nothing
// is written.
type ReferenceRespondResult struct {
	Status    string           `json:"status"`
	Reference ReferenceRequest `json:"reference"`
	EventID   *string          `json:"eventId"`
}

// ─── Types: threshold policies ───────────────────────────────────────────────

// ThresholdPolicy is a declarative decision trigger (the object under the
// "policy" key). UserID is your own external id for a subject-scoped policy, or
// nil for an appliesToAll policy.
type ThresholdPolicy struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	AppliesToAll    bool     `json:"appliesToAll"`
	Metric          string   `json:"metric"`
	Direction       *string  `json:"direction"`
	Threshold       *float64 `json:"threshold"`
	Component       *string  `json:"component"`
	Band            *string  `json:"band"`
	IsActive        bool     `json:"isActive"`
	LastTriggeredAt *string  `json:"lastTriggeredAt"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
	UserID          *string  `json:"userId"`
}

// CreatePolicyInput is the body for CreatePolicy. Set exactly one of UserID
// (watch one subject) or AppliesToAll (watch all your subjects). Metric is one
// of "score" / "component" / "band" / "verified_events"; supply the condition
// its metric requires (Direction — "up"/"down" for crossings, "enter"/"leave"
// for bands — plus Threshold / Component / Band).
type CreatePolicyInput struct {
	Name         string   `json:"name"`
	UserID       string   `json:"userId,omitempty"`
	AppliesToAll *bool    `json:"appliesToAll,omitempty"`
	Metric       string   `json:"metric"`
	Direction    string   `json:"direction,omitempty"`
	Threshold    *float64 `json:"threshold,omitempty"`
	Component    string   `json:"component,omitempty"`
	Band         string   `json:"band,omitempty"`
}

// UpdatePolicyInput patches a policy. The metric is immutable. Nil fields are
// omitted; use the nullable pointer fields to clear a condition value.
type UpdatePolicyInput struct {
	Name      *string  `json:"name,omitempty"`
	Direction *string  `json:"direction,omitempty"`
	Threshold *float64 `json:"threshold,omitempty"`
	Component *string  `json:"component,omitempty"`
	Band      *string  `json:"band,omitempty"`
	IsActive  *bool    `json:"isActive,omitempty"`
}

// ThresholdPolicyResult is the envelope returned by CreatePolicy / GetPolicy /
// UpdatePolicy.
type ThresholdPolicyResult struct {
	Policy ThresholdPolicy `json:"policy"`
}

// ThresholdPolicyListResult is the cursor-paginated envelope from ListPolicies.
type ThresholdPolicyListResult struct {
	Data       []ThresholdPolicy `json:"data"`
	NextCursor *string           `json:"nextCursor"`
}

// ─── Types: Open Badges 3.0 ─────────────────────────────────────────────────

// OpenBadgeAchievementsPayload is GET /api/v1/open-badges/achievements — the
// closed, signable set. Achievements follow the Open Badges 3.0 Achievement
// shape and are left loosely typed.
type OpenBadgeAchievementsPayload struct {
	Specification  map[string]any   `json:"specification"`
	Note           string           `json:"note"`
	AchievementIDs []string         `json:"achievementIds"`
	Achievements   []map[string]any `json:"achievements"`
}

// ─── Public reads (no API key) ──────────────────────────────────────────────

// GetBenchmarks fetches the public benchmark catalog — cohort dimensions and the
// k-anonymity floor. GET /api/v1/benchmarks. No API key required. A benchmark is
// a distribution fact, never a verdict.
func (c *Client) GetBenchmarks(ctx context.Context) (*BenchmarkCatalog, error) {
	var out BenchmarkCatalog
	if err := c.get(ctx, "/benchmarks", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOpenBadgeAchievements fetches the closed set of Open Badges 3.0 achievements
// the Credda issuer key will sign. GET /api/v1/open-badges/achievements. Public,
// no key — every signed credential's achievement.id resolves here.
func (c *Client) GetOpenBadgeAchievements(ctx context.Context) (*OpenBadgeAchievementsPayload, error) {
	var out OpenBadgeAchievementsPayload
	if err := c.get(ctx, "/open-badges/achievements", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOpenBadgeAchievement fetches one Open Badges 3.0 achievement definition by
// id. GET /api/v1/open-badges/achievements/:badgeId. Public, no key. The shape
// is an Open Badges Achievement object, returned as a generic map.
func (c *Client) GetOpenBadgeAchievement(ctx context.Context, badgeID string) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/open-badges/achievements/"+esc(badgeID), false, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetCredentialIssuerMetadata fetches the OID4VCI issuer discovery document — the
// doc a wallet reads first. GET /.well-known/openid-credential-issuer. Public, no
// key. Minting the offer that starts the flow is a keyed call — see
// CreateCredentialOffer.
func (c *Client) GetCredentialIssuerMetadata(ctx context.Context) (*CredentialIssuerMetadata, error) {
	var out CredentialIssuerMetadata
	if err := c.getWellKnown(ctx, "/.well-known/openid-credential-issuer", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Platform reads (API key required) ──────────────────────────────────────

// ListUsers queries and exports your book of subjects — a cursor-paginated page
// of the subjects you have reported events for, with a closed filter set. Pass
// nil for no filters. GET /api/v1/users. Requires scores:read; a test key lists
// only the test population.
func (c *Client) ListUsers(ctx context.Context, query *ListUsersQuery) (*ListUsersPayload, error) {
	qs := url.Values{}
	if query != nil {
		applyBookFilters(qs, &query.BookFilterQuery)
		setStr(qs, "sort", query.Sort)
		setStr(qs, "order", query.Order)
		setInt(qs, "limit", query.Limit)
		setStr(qs, "cursor", query.Cursor)
	}
	var out ListUsersPayload
	if err := c.get(ctx, withQuery("/users", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// applyBookFilters serialises the closed book filter set. Shared by ListUsers
// and GetBookSummary so the two surfaces can never disagree about the filter
// vocabulary — the same reason the server parses both with one function.
func applyBookFilters(qs url.Values, f *BookFilterQuery) {
	if f == nil {
		return
	}
	setFloat(qs, "scoreMin", f.ScoreMin)
	setFloat(qs, "scoreMax", f.ScoreMax)
	setStr(qs, "band", f.Band)
	setBool(qs, "hasScore", f.HasScore)
	setBool(qs, "scoreFrozen", f.ScoreFrozen)
	setStr(qs, "subjectType", f.SubjectType)
	setStr(qs, "activeSince", f.ActiveSince)
	setStr(qs, "registeredSince", f.RegisteredSince)
	setStr(qs, "registeredBefore", f.RegisteredBefore)
	setBool(qs, "hasVerifiedEvents", f.HasVerifiedEvents)
	setInt(qs, "minVerifiedEvents", f.MinVerifiedEvents)
}

// GetBookSummary sizes and shapes a segment of your book WITHOUT paging it: how
// many subjects match, how many are scored, their band mix and median/mean.
// Takes the same closed filter set as ListUsers and is built from the identical
// tenant-scoped query, so it can never count a subject the listing would not
// show you. Pass nil for the whole book. GET /api/v1/users/summary. Requires
// scores:read; read-only.
//
// Nothing is faked: Central's members are nil when nothing in the segment is
// scored, and an oversized population returns the exact Matched count with nil
// aggregates plus an AggregationSkipped reason.
func (c *Client) GetBookSummary(ctx context.Context, filters *BookFilterQuery) (*BookSummaryPayload, error) {
	qs := url.Values{}
	applyBookFilters(qs, filters)
	var out BookSummaryPayload
	if err := c.get(ctx, withQuery("/users/summary", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTrustSummary returns a deterministic, evidence-based trust summary for a
// subject — summary + strengths + risks drawn only from recorded evidence, with
// a standing advisory that this is evidence, not a recommendation. Pass
// narrative=true to also attach an advisory AI retelling (inert unless the
// server has AI configured). GET /api/v1/users/:id/trust-summary.
func (c *Client) GetTrustSummary(ctx context.Context, userID string, narrative bool) (*TrustSummaryPayload, error) {
	qs := url.Values{}
	if narrative {
		qs.Set("narrative", "1")
	}
	var out TrustSummaryPayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/trust-summary", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBenchmarkDistribution returns the aggregate score distribution for a cohort
// (median/mean/p25/p75/p90 + band histogram). Pass an empty dimension for "all";
// pass an empty cohort to get every cohort value on the dimension. Any cohort
// below the k-anonymity floor comes back Available=false with no numbers.
// GET /api/v1/benchmarks/distribution. Requires scores:read.
func (c *Client) GetBenchmarkDistribution(ctx context.Context, dimension, cohort string) (*BenchmarkDistributionPayload, error) {
	qs := url.Values{}
	setStr(qs, "dimension", dimension)
	setStr(qs, "cohort", cohort)
	var out BenchmarkDistributionPayload
	if err := c.get(ctx, withQuery("/benchmarks/distribution", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUserBenchmark returns where a single subject sits within a cohort — their
// percentile rank plus the cohort's aggregate distribution. Available=false with
// Reason "insufficient_data" (cohort below the floor) or "no_score" (subject not
// scored yet). Pass an empty dimension for the default. This is the REAL
// comparison, distinct from the deprecated Score.percentile (100 − score). A
// percentile is not a verdict. GET /api/v1/users/:id/benchmark.
func (c *Client) GetUserBenchmark(ctx context.Context, userID, dimension string) (*UserBenchmarkPayload, error) {
	qs := url.Values{}
	setStr(qs, "dimension", dimension)
	var out UserBenchmarkPayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/benchmark", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Writes (API key required) ──────────────────────────────────────────────

// CreateCredentialOffer mints an OID4VCI credential offer to start the wallet
// issuance flow. Render CredentialOfferURI as a QR code or link; the wallet then
// completes the token / nonce / credential exchange itself. Leave the input's
// fields empty to offer the issuer default. POST /api/v1/users/:id/credential-offer.
func (c *Client) CreateCredentialOffer(ctx context.Context, userID string, input CredentialOfferInput) (*CredentialOfferResult, error) {
	var out CredentialOfferResult
	if err := c.post(ctx, "/users/"+esc(userID)+"/credential-offer", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReplayWebhookDelivery re-sends a stored delivery with its ORIGINAL event id and
// a fresh signature (current secret + fresh transport timestamp), so the replay
// verifies at the receiver. Deliveries older than the retention window return
// 409 PAYLOAD_NOT_RETAINED.
// POST /api/v1/webhooks/:id/deliveries/:deliveryId/replay.
func (c *Client) ReplayWebhookDelivery(ctx context.Context, webhookID, deliveryID string) (*WebhookReplayResult, error) {
	var out WebhookReplayResult
	if err := c.post(ctx, "/webhooks/"+esc(webhookID)+"/deliveries/"+esc(deliveryID)+"/replay", map[string]any{}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Confirmation requests (counterparty-confirmation primitive) ────────────

// CreateConfirmationRequest proposes an outcome for counterparty confirmation. It
// writes NO event and touches NO score; it returns a one-time confirmation token
// (shown ONCE — deliver it to the counterparty yourself) plus preview/respond
// URLs. Pass a stable idempotencyKey (empty string to omit) to make retries
// exactly-once. POST /api/v1/confirmations. Requires events:write.
func (c *Client) CreateConfirmationRequest(ctx context.Context, input CreateConfirmationInput, idempotencyKey string) (*ConfirmationCreateResult, error) {
	var headers map[string]string
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	var out ConfirmationCreateResult
	if err := c.post(ctx, "/confirmations", input, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateConfirmationBatch is the ACTIVATION ENGINE — bulk-create up to 100
// confirmation requests in one call. Turn your BOOK of historical relationships
// (past jobs, placements, engagements, projects) into pending counterparty asks,
// warming a cold ledger. Each item is exactly a CreateConfirmationInput and flows
// through the SAME create path, so isVerified is still earned only on confirm — a
// batch item writes NOTHING to the ledger until its named counterparty confirms.
//
// Partial success: Results lists each item's outcome by Index — an ok item carries
// its one-time token + hosted confirmUrl; a failed one carries the reason + code
// (e.g. CONFIRMATION_SELF). Pass a stable idempotencyKey (empty string to omit) so
// a retried batch replays instead of duplicating. POST /api/v1/confirmations/batch.
// Requires events:write.
func (c *Client) CreateConfirmationBatch(ctx context.Context, requests []CreateConfirmationInput, idempotencyKey string) (*ConfirmationBatchResult, error) {
	var headers map[string]string
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	var out ConfirmationBatchResult
	if err := c.post(ctx, "/confirmations/batch", map[string]any{"requests": requests}, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListConfirmations lists your confirmation requests, newest first,
// cursor-paginated. Pass an empty status to list all. GET /api/v1/confirmations.
func (c *Client) ListConfirmations(ctx context.Context, status string, limit *int, cursor string) (*ConfirmationListResult, error) {
	qs := url.Values{}
	setStr(qs, "status", status)
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out ConfirmationListResult
	if err := c.get(ctx, withQuery("/confirmations", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetConfirmation fetches one of your confirmation requests.
// GET /api/v1/confirmations/:id.
func (c *Client) GetConfirmation(ctx context.Context, id string) (*ConfirmationResult, error) {
	var out ConfirmationResult
	if err := c.get(ctx, "/confirmations/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelConfirmation cancels a still-pending confirmation request. A request that
// is already decided/expired returns 409 CONFIRMATION_NOT_PENDING.
// POST /api/v1/confirmations/:id/cancel.
func (c *Client) CancelConfirmation(ctx context.Context, id string) (*ConfirmationResult, error) {
	var out ConfirmationResult
	if err := c.post(ctx, "/confirmations/"+esc(id)+"/cancel", map[string]any{}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PreviewConfirmation returns what the counterparty is being asked to confirm — a
// PII-free subset (never the raw subject id). Token-gated; NO API key.
// GET /api/v1/confirmations/:id/preview?token=….
func (c *Client) PreviewConfirmation(ctx context.Context, id, token string) (*ConfirmationPreviewResult, error) {
	qs := url.Values{}
	qs.Set("token", token)
	var out ConfirmationPreviewResult
	if err := c.get(ctx, withQuery("/confirmations/"+esc(id)+"/preview", qs), false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RespondToConfirmation submits the counterparty's decision, presented with the
// raw token — NO API key. decision is "confirm" (the proposed event is written,
// verified, and EventID is returned) or "decline" (nothing is written).
// Single-use. POST /api/v1/confirmations/:id/respond.
func (c *Client) RespondToConfirmation(ctx context.Context, id, token, decision string) (*ConfirmationRespondResult, error) {
	body := map[string]any{"token": token, "decision": decision}
	var out ConfirmationRespondResult
	if err := c.postPublic(ctx, "/confirmations/"+esc(id)+"/respond", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Reference / employment-verification requests ───────────────────────────

// CreateReferenceRequest proposes a résumé claim (employment / education /
// certification / skill) for the named third party who was there to confirm. It
// records NO qualification and touches NO score; it returns a one-time reference
// token (shown ONCE — deliver it to the reference yourself) plus preview/respond
// URLs. Pass a stable idempotencyKey (empty string to omit) to make retries
// exactly-once. POST /api/v1/references. Requires events:write.
func (c *Client) CreateReferenceRequest(ctx context.Context, input CreateReferenceInput, idempotencyKey string) (*ReferenceCreateResult, error) {
	var headers map[string]string
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	var out ReferenceCreateResult
	if err := c.post(ctx, "/references", input, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListReferences lists your reference requests, newest first, cursor-paginated.
// Pass an empty status to list all. GET /api/v1/references.
func (c *Client) ListReferences(ctx context.Context, status string, limit *int, cursor string) (*ReferenceListResult, error) {
	qs := url.Values{}
	setStr(qs, "status", status)
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out ReferenceListResult
	if err := c.get(ctx, withQuery("/references", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetReference fetches one of your reference requests. GET /api/v1/references/:id.
func (c *Client) GetReference(ctx context.Context, id string) (*ReferenceResult, error) {
	var out ReferenceResult
	if err := c.get(ctx, "/references/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelReference cancels a still-pending reference request. A request that is
// already decided/expired returns 409 REFERENCE_NOT_PENDING.
// POST /api/v1/references/:id/cancel.
func (c *Client) CancelReference(ctx context.Context, id string) (*ReferenceResult, error) {
	var out ReferenceResult
	if err := c.post(ctx, "/references/"+esc(id)+"/cancel", map[string]any{}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PreviewReference returns what the counterparty is being asked to confirm — a
// PII-free subset (never the raw subject id or the counterpartyRef matching
// key). Token-gated; NO API key. GET /api/v1/references/:id/preview?token=….
func (c *Client) PreviewReference(ctx context.Context, id, token string) (*ReferencePreviewResult, error) {
	qs := url.Values{}
	qs.Set("token", token)
	var out ReferencePreviewResult
	if err := c.get(ctx, withQuery("/references/"+esc(id)+"/preview", qs), false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RespondToReference submits the reference's decision, presented with the raw
// token — NO API key. decision is "confirm" (the qualification is recorded,
// verified, and EventID is returned) or "decline" (nothing is written).
// Single-use. A qualification never moves the reliability score.
// POST /api/v1/references/:id/respond.
func (c *Client) RespondToReference(ctx context.Context, id, token, decision string) (*ReferenceRespondResult, error) {
	body := map[string]any{"token": token, "decision": decision}
	var out ReferenceRespondResult
	if err := c.postPublic(ctx, "/references/"+esc(id)+"/respond", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Threshold policies (declarative decision triggers) ─────────────────────

// CreatePolicy creates a threshold policy — a "notify me when a subject crosses
// THIS line" rule that delivers a policy.threshold_crossed webhook. A policy
// never reads into, blocks, or changes a score. POST /api/v1/policies. Uses the
// webhooks scope.
func (c *Client) CreatePolicy(ctx context.Context, input CreatePolicyInput) (*ThresholdPolicyResult, error) {
	var out ThresholdPolicyResult
	if err := c.post(ctx, "/policies", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPolicies lists this platform's threshold policies, cursor-paginated.
// GET /api/v1/policies.
func (c *Client) ListPolicies(ctx context.Context, limit *int, cursor string) (*ThresholdPolicyListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out ThresholdPolicyListResult
	if err := c.get(ctx, withQuery("/policies", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPolicy fetches one policy. GET /api/v1/policies/:id.
func (c *Client) GetPolicy(ctx context.Context, id string) (*ThresholdPolicyResult, error) {
	var out ThresholdPolicyResult
	if err := c.get(ctx, "/policies/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePolicy retunes a policy (name / direction / threshold / component / band
// / isActive). The metric is immutable — to change it, delete and recreate.
// PATCH /api/v1/policies/:id.
func (c *Client) UpdatePolicy(ctx context.Context, id string, patch UpdatePolicyInput) (*ThresholdPolicyResult, error) {
	var out ThresholdPolicyResult
	if err := c.patch(ctx, "/policies/"+esc(id), patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePolicy deletes a policy (hard delete — it is config, not ledger data).
// DELETE /api/v1/policies/:id.
func (c *Client) DeletePolicy(ctx context.Context, id string) error {
	return c.delete(ctx, "/policies/"+esc(id))
}

// ─── Types: API version contract + changelog ─────────────────────────────────

// ChangelogEntry is one dated change from GET /api/v1/changelog.
type ChangelogEntry struct {
	ID string `json:"id"`
	// Date is an ISO date (YYYY-MM-DD) — the release date, because the API
	// deploys on merge.
	Date string `json:"date"`
	// Category is one of "added" / "changed" / "deprecated" / "fixed" / "security".
	Category string `json:"category"`
	Summary  string `json:"summary"`
	// Endpoints are OpenAPI-style paths the change touched.
	Endpoints []string `json:"endpoints,omitempty"`
	Reference string   `json:"reference,omitempty"`
}

// DeprecationNotice is a scheduled removal. The list is empty while nothing is
// deprecated — which is not the same as unavailable.
type DeprecationNotice struct {
	Path        string   `json:"path"`
	Methods     []string `json:"methods,omitempty"`
	AnnouncedAt string   `json:"announcedAt"`
	// SunsetAt is the RFC 8594 Sunset date, as an ISO timestamp.
	SunsetAt    string `json:"sunsetAt"`
	Replacement string `json:"replacement"`
	InfoURL     string `json:"infoUrl,omitempty"`
}

// DeprecationPolicy is the deprecation contract, including the wire signalling.
type DeprecationPolicy struct {
	MinimumNoticeDays  int    `json:"minimumNoticeDays"`
	Announcement       string `json:"announcement"`
	Headers            string `json:"headers"`
	BehaviourUnchanged string `json:"behaviourUnchanged"`
	ActiveCount        int    `json:"activeCount"`
}

// VersioningContract says exactly what "v1 is additive-only" guarantees: what can
// appear without notice and what would require a new major version.
type VersioningContract struct {
	Version   string `json:"version"`
	Scheme    string `json:"scheme"`
	Guarantee string `json:"guarantee"`
	// Additive changes can ship at any time inside v1, without notice.
	Additive []string `json:"additive"`
	// Breaking changes would require a new major version. None has been made.
	Breaking []string `json:"breaking"`
	// BehaviourVersions is the honest caveat: the API SHAPE is versioned by v1,
	// but what the API computes carries its own versions (see ComponentVersions).
	BehaviourVersions string            `json:"behaviourVersions"`
	ComponentVersions map[string]string `json:"componentVersions"`
	NextMajorVersion  string            `json:"nextMajorVersion"`
	Deprecation       DeprecationPolicy `json:"deprecation"`
}

// APIChangelog is GET /api/v1/changelog — the version contract plus every dated
// change, newest first.
type APIChangelog struct {
	APIVersion   string              `json:"apiVersion"`
	Note         string              `json:"note"`
	Versioning   VersioningContract  `json:"versioning"`
	Deprecations []DeprecationNotice `json:"deprecations"`
	Categories   []string            `json:"categories"`
	LatestChange *string             `json:"latestChange"`
	Count        int                 `json:"count"`
	Entries      []ChangelogEntry    `json:"entries"`
}

// ─── Types: verified profile (qualifications) ────────────────────────────────
//
// A SECOND deterministic measure over the same ledger: how much of a person's
// CLAIMED record (education, skills, certifications, employment) is
// independently third-party verified.
//
// THE BRIGHT LINE: this can never move the Reliability Score — qualification
// events are structurally excluded from the score formula. It counts WHETHER a
// claim is verified, never how prestigious it is: no school, employer, degree or
// credential is ranked or weighted, deliberately.

// QualificationBreakdown is the per-category claimed vs verified count.
type QualificationBreakdown struct {
	Claimed  int `json:"claimed"`
	Verified int `json:"verified"`
	// VerificationDepth is verified ÷ claimed. Nil — never 0 — when nothing is
	// claimed in this category.
	VerificationDepth *float64 `json:"verificationDepth"`
}

// QualificationTotals is the whole-record tally.
type QualificationTotals struct {
	Claimed  int `json:"claimed"`
	Verified int `json:"verified"`
	// SelfAttested are claims recorded but not yet independently verified.
	SelfAttested int `json:"selfAttested"`
}

// VerifiedProfilePayload is GET /api/v1/users/:id/verified-profile.
type VerifiedProfilePayload struct {
	UserID         string `json:"userId"`
	ProfileVersion string `json:"profileVersion"`
	// Categories is keyed by "education" / "skill" / "certification" / "employment".
	Categories map[string]QualificationBreakdown `json:"categories"`
	Totals     QualificationTotals               `json:"totals"`
	// VerificationDepth is the share of the WHOLE claimed record that is
	// independently verified. Equal weight per claim — no prestige, no ranking.
	// Nil when nothing is claimed.
	VerificationDepth *float64 `json:"verificationDepth"`
	Note              string   `json:"note"`
	// Disclosures state what this measure is not. Always present.
	Disclosures []string `json:"disclosures"`
}

// RecordQualificationInput is the body for RecordQualification. Category is
// required and is one of "education" / "skill" / "certification" / "employment".
type RecordQualificationInput struct {
	Category string `json:"category"`
	// Label is a free-text claim label. Carried for display; never read by the
	// measure.
	Label string `json:"label,omitempty"`
	// Issuer is a free-text institution/employer. Carried for display; NEVER ranked.
	Issuer string `json:"issuer,omitempty"`
	// VerifiedBy names the third-party witness. Required for the claim to count
	// as verified — there is deliberately no "isVerified" field to set.
	VerifiedBy string `json:"verifiedBy,omitempty"`
}

// RecordQualificationResult is POST /api/v1/users/:id/qualifications. The claim
// is always recorded; IsVerified is decided by the witness rule.
type RecordQualificationResult struct {
	UserID     string `json:"userId"`
	EventID    string `json:"eventId"`
	Category   string `json:"category"`
	EventType  string `json:"eventType"`
	IsVerified bool   `json:"isVerified"`
	// VerificationNote says why the claim was recorded as self-attested. Nil
	// when verified.
	VerificationNote *string `json:"verificationNote"`
	Note             string  `json:"note"`
}

// ─── Types: professional record ──────────────────────────────────────────────
//
// A worker-OWNED, résumé-shaped summary of a VERIFIED work record: reliability
// band, verified-outcome counts, verification depth, tenure. Pure derivation over
// the ledger the score already reads — no new scoring logic, nothing here can
// move a score.
//
// It describes the record the subject chose to present. It is NOT a hiring,
// promotion or employment recommendation, NOT a background check, and NOT a
// consumer report. The disclosures travel on every payload for that reason.

// ProfessionalRecordReliability is the reliability figure the record rests on.
type ProfessionalRecordReliability struct {
	Score float64 `json:"score"`
	Band  string  `json:"band"`
	// Confidence is verified-evidence confidence, 0..1.
	Confidence float64 `json:"confidence"`
}

// ProfessionalRecordExperience counts only third-party-verified outcomes as
// verified experience.
type ProfessionalRecordExperience struct {
	VerifiedOutcomes int `json:"verifiedOutcomes"`
	TotalOutcomes    int `json:"totalOutcomes"`
	// VerificationDepth is verified ÷ total. Nil — the honest answer, not 0 —
	// when there is no record yet.
	VerificationDepth *float64 `json:"verificationDepth"`
	VerifiedPlatforms int      `json:"verifiedPlatforms"`
}

// ProfessionalRecordTenure is the OBSERVED span of the record. A missing figure
// is nil, never a default — nothing is extrapolated.
type ProfessionalRecordTenure struct {
	FirstRecordedAt *string `json:"firstRecordedAt"`
	FirstVerifiedAt *string `json:"firstVerifiedAt"`
	LastRecordedAt  *string `json:"lastRecordedAt"`
	// TrackRecordDays is whole days from first to last recorded outcome.
	TrackRecordDays   *int     `json:"trackRecordDays"`
	TrackRecordMonths *float64 `json:"trackRecordMonths"`
}

// ProfessionalRecordStatus reports whether the underlying score is frozen.
type ProfessionalRecordStatus struct {
	ScoreFrozen bool `json:"scoreFrozen"`
}

// ProfessionalRecordProvenance records which formula produced the figures.
type ProfessionalRecordProvenance struct {
	FormulaVersion string  `json:"formulaVersion"`
	ComputedAt     *string `json:"computedAt"`
}

// ProfessionalRecord is the derived summary — also the block embedded in the
// public verify payload.
type ProfessionalRecord struct {
	ProfessionalRecordVersion string                        `json:"professionalRecordVersion"`
	Note                      string                        `json:"note"`
	Reliability               ProfessionalRecordReliability `json:"reliability"`
	VerifiedExperience        ProfessionalRecordExperience  `json:"verifiedExperience"`
	Tenure                    ProfessionalRecordTenure      `json:"tenure"`
	Status                    ProfessionalRecordStatus      `json:"status"`
	Provenance                ProfessionalRecordProvenance  `json:"provenance"`
	// Disclosures include that this is not a hiring decision or a consumer report.
	Disclosures []string `json:"disclosures"`
}

// ProfessionalRecordPayload is GET /api/v1/users/:id/professional-record.
type ProfessionalRecordPayload struct {
	ProfessionalRecord
	UserID string `json:"userId"`
}

// PublicProfessionalRecordPayload is
// GET /api/v1/verify/:token?scope=full&professional=1 — the public trust payload
// with the professional-record block attached. ProfessionalRecord is nil if it
// cannot be derived (the block is fail-safe).
type PublicProfessionalRecordPayload struct {
	TrustPayload
	Scope              string              `json:"scope"`
	CredentialScope    string              `json:"credentialScope,omitempty"`
	ProfessionalRecord *ProfessionalRecord `json:"professionalRecord"`
}

// ─── Types: worker reliability report ────────────────────────────────────────

// ReliabilityReportOutcome is one recent outcome in the report — flagged verified
// vs self-reported.
type ReliabilityReportOutcome struct {
	EventType  string `json:"eventType"`
	Stake      string `json:"stake"`
	Verified   bool   `json:"verified"`
	Source     string `json:"source"` // "verified" | "self_reported"
	OccurredAt string `json:"occurredAt"`
}

// ReliabilityReportFactor is a ranked driver of the score (a relabelled reason
// code).
type ReliabilityReportFactor struct {
	Code         string  `json:"code"`
	Factor       string  `json:"factor"`
	Direction    string  `json:"direction"` // "adverse" | "supporting"
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Contribution float64 `json:"contribution"`
	Rank         int     `json:"rank"`
}

// ReliabilityReportReliability is the score the report rests on, with the versions
// that produced it.
type ReliabilityReportReliability struct {
	Score              float64 `json:"score"`
	Band               string  `json:"band"`
	Confidence         float64 `json:"confidence"`
	FormulaVersion     string  `json:"formulaVersion"`
	ReasonCodesVersion string  `json:"reasonCodesVersion"`
}

// ReliabilityReportMetrics are the underlying rates. Recency is nil when the
// record has no dated activity — never 0.
type ReliabilityReportMetrics struct {
	CompletionRate float64  `json:"completionRate"`
	OnTimeRate     float64  `json:"onTimeRate"`
	Consistency    float64  `json:"consistency"`
	Recency        *float64 `json:"recency"`
	DisputeRate    float64  `json:"disputeRate"`
}

// ReliabilityReportExperience is the professional-record verified-experience block
// with tenure attached.
type ReliabilityReportExperience struct {
	VerifiedOutcomes  int                      `json:"verifiedOutcomes"`
	TotalOutcomes     int                      `json:"totalOutcomes"`
	VerificationDepth *float64                 `json:"verificationDepth"`
	VerifiedPlatforms int                      `json:"verifiedPlatforms"`
	Tenure            ProfessionalRecordTenure `json:"tenure"`
}

// ReliabilityReportBenchmark is the coarse cohort comparison — nil unless
// requested and available.
type ReliabilityReportBenchmark struct {
	Cohort     string `json:"cohort"`
	Comparison string `json:"comparison"`
}

// ReliabilityReport is the consolidated decision-support dossier — an AGGREGATION
// of already-computed values that carries no new score.
//
// It is EVIDENCE a reader weighs against their own criteria — NOT a hire / place /
// rank / approve verdict, a background check, or a consumer report. The
// disclosures travel on every payload.
type ReliabilityReport struct {
	ReliabilityReportVersion string                       `json:"reliabilityReportVersion"`
	Note                     string                       `json:"note"`
	Reliability              ReliabilityReportReliability `json:"reliability"`
	Metrics                  ReliabilityReportMetrics     `json:"metrics"`
	VerifiedExperience       ReliabilityReportExperience  `json:"verifiedExperience"`
	TopFactors               []ReliabilityReportFactor    `json:"topFactors"`
	RecentOutcomes           []ReliabilityReportOutcome   `json:"recentOutcomes"`
	Benchmark                *ReliabilityReportBenchmark  `json:"benchmark"`
	Status                   ProfessionalRecordStatus     `json:"status"`
	Provenance               ProfessionalRecordProvenance `json:"provenance"`
	Disclosures              []string                     `json:"disclosures"`
	Advisory                 string                       `json:"advisory"`
}

// ReliabilityReportPayload is GET /api/v1/users/:id/reliability-report.
type ReliabilityReportPayload struct {
	ReliabilityReport
	UserID string `json:"userId"`
}

// PublicReliabilityReportPayload is
// GET /api/v1/verify/:token/reliability-report — the worker-consent variant.
// ReliabilityReport is nil if it cannot be derived (the block is fail-safe).
type PublicReliabilityReportPayload struct {
	Token             string             `json:"token"`
	Issuer            string             `json:"issuer"`
	ReliabilityReport *ReliabilityReport `json:"reliabilityReport"`
}

// LinkedInCertification is the "Add to LinkedIn" certification deep link.
// LinkedIn stores only the name, organization, dates, credential id and CertURL —
// it does not import credential claims, and Note says so.
type LinkedInCertification struct {
	AddToProfileURL string `json:"addToProfileUrl"`
	CertURL         string `json:"certUrl"`
	CertID          string `json:"certId"`
	Note            string `json:"note"`
}

// ProfessionalRecordCredentialResult is
// POST /api/v1/users/:id/professional-record/credential — a signed, offline-
// verifiable W3C VC-JWT on the same Ed25519 issuer key / did:web /
// StatusList2021 revocation as every other Credda credential.
type ProfessionalRecordCredentialResult struct {
	Format                    string                `json:"format"` // "jwt_vc_json"
	CredentialVc              string                `json:"credentialVc"`
	CredentialType            string                `json:"credentialType"` // "CreddaProfessionalRecordCredential"
	Issuer                    string                `json:"issuer"`
	Kid                       string                `json:"kid"`
	Scope                     string                `json:"scope"`
	ProfessionalRecordVersion string                `json:"professionalRecordVersion"`
	Claims                    map[string]any        `json:"claims"`
	IssuedAt                  string                `json:"issuedAt"`
	ExpiresAt                 string                `json:"expiresAt"`
	LinkedIn                  LinkedInCertification `json:"linkedin"`
	DIDDocument               string                `json:"didDocument"`
	TrustRegistry             string                `json:"trustRegistry"`
	StatusList                string                `json:"statusList"`
}

// ─── Public reads (no API key) ──────────────────────────────────────────────

// GetChangelog fetches the API version contract and the dated changelog.
// GET /api/v1/changelog. Public, no key.
//
// Versioning says exactly what "v1 is additive-only" guarantees — what can appear
// without notice (new endpoints, response fields, optional inputs, enum values,
// error codes, webhook event types) and what would require a new major version.
// Deprecations is empty while nothing is deprecated; a deprecated endpoint
// additionally answers with Deprecation (RFC 9745) and Sunset (RFC 8594)
// headers. Entries are newest-first.
func (c *Client) GetChangelog(ctx context.Context) (*APIChangelog, error) {
	var out APIChangelog
	if err := c.get(ctx, "/changelog", false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPublicProfessionalRecord fetches the subject's PROFESSIONAL RECORD behind a
// public share token, alongside the usual public trust payload.
// GET /api/v1/verify/:token?scope=full&professional=1. No API key — the token is
// the subject's own consent to present it.
//
// Requests scope=full because the API serves the record block ONLY at full
// disclosure: a band/minimal embed must never carry it. The block is fail-safe
// nil if it cannot be derived.
//
// It describes a record the subject chose to present. It is NOT a hiring verdict,
// a background check, or a consumer report.
func (c *Client) GetPublicProfessionalRecord(ctx context.Context, token string) (*PublicProfessionalRecordPayload, error) {
	qs := url.Values{}
	qs.Set("scope", "full")
	qs.Set("professional", "1")
	var out PublicProfessionalRecordPayload
	if err := c.get(ctx, withQuery("/verify/"+esc(token), qs), false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPublicReliabilityReport fetches the WORKER RELIABILITY REPORT behind a public
// share token. GET /api/v1/verify/:token/reliability-report. Public — NO API key:
// the token is the worker's own consent to hand their dossier to a prospective
// employer. Pass a non-nil recent (1–50) to bound the recent-outcomes list and
// benchmark=true to attach the coarse quartile-grain comparison.
//
// ReliabilityReport is nil if it cannot be derived (fail-safe). It is EVIDENCE a
// reader weighs against their own criteria — NOT a hire / place / rank / approve
// verdict, a background check, or a consumer report.
func (c *Client) GetPublicReliabilityReport(ctx context.Context, token string, recent *int, benchmark bool) (*PublicReliabilityReportPayload, error) {
	qs := url.Values{}
	setInt(qs, "recent", recent)
	if benchmark {
		qs.Set("benchmark", "1")
	}
	var out PublicReliabilityReportPayload
	if err := c.get(ctx, withQuery("/verify/"+esc(token)+"/reliability-report", qs), false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Platform reads / writes (API key required) ─────────────────────────────

// GetVerifiedProfile fetches the subject's verified-profile measure: per-category
// claimed vs verified counts, and the share of the whole claimed record that is
// independently verified. GET /api/v1/users/:id/verified-profile.
//
// VerificationDepth is nil — not 0 — when nothing is claimed. This describes what
// is verified; it is not an assessment of the person.
func (c *Client) GetVerifiedProfile(ctx context.Context, userID string) (*VerifiedProfilePayload, error) {
	var out VerifiedProfilePayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/verified-profile", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RecordQualification records a qualification claim.
// POST /api/v1/users/:id/qualifications.
//
// The claim is ALWAYS recorded. Whether it counts as VERIFIED is decided by the
// witness rule, never by you: set VerifiedBy to the genuine third party that
// confirmed it. Absent (or naming the subject themselves) the claim still lands
// on the ledger but as self-attested, with VerificationNote saying why — it does
// not raise verification depth.
//
// Issuer/Label are carried for display and are never read by the measure. Writes
// nothing score-side and never enqueues a recompute.
func (c *Client) RecordQualification(ctx context.Context, userID string, input RecordQualificationInput) (*RecordQualificationResult, error) {
	var out RecordQualificationResult
	if err := c.post(ctx, "/users/"+esc(userID)+"/qualifications", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetProfessionalRecord fetches the subject's professional record.
// GET /api/v1/users/:id/professional-record. Only third-party-verified outcomes
// count as verified experience; tenure is the OBSERVED span of the record and a
// missing figure is nil, never a default. Nothing is extrapolated.
func (c *Client) GetProfessionalRecord(ctx context.Context, userID string) (*ProfessionalRecordPayload, error) {
	var out ProfessionalRecordPayload
	if err := c.get(ctx, "/users/"+esc(userID)+"/professional-record", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetReliabilityReport fetches the consolidated WORKER RELIABILITY REPORT — the
// buy-trigger read a staffing agency or employer weighs before placing or hiring.
// GET /api/v1/users/:id/reliability-report (requires a platform API key). It is an
// AGGREGATION of what the engine already computed (reliability, metrics, verified
// experience + tenure, ranked drivers, recent outcomes, an optional coarse
// benchmark) and computes no new score. Pass a non-nil recent (1–50, default 10)
// to bound the outcomes list and benchmark=true to attach the comparison.
//
// Every recent outcome is flagged verified vs self_reported; self-reported
// activity is never presented as verified. It is EVIDENCE a reader weighs against
// their own criteria — NOT a hire / place / rank / approve verdict, a background
// check, or a consumer report.
func (c *Client) GetReliabilityReport(ctx context.Context, userID string, recent *int, benchmark bool) (*ReliabilityReportPayload, error) {
	qs := url.Values{}
	setInt(qs, "recent", recent)
	if benchmark {
		qs.Set("benchmark", "1")
	}
	var out ReliabilityReportPayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/reliability-report", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MintProfessionalRecordCredential mints a signed Professional Record Credential
// (W3C VC-JWT, type CreddaProfessionalRecordCredential) so the subject can PROVE
// their verified record without the verifier calling Credda.
// POST /api/v1/users/:id/professional-record/credential. Pass nil ttlSeconds for
// the default lifetime.
//
// The credential rides the same Ed25519 issuer key / did:web / StatusList2021
// revocation as every other Credda VC — verify it with any JOSE library (see the
// README note on offline verification).
//
// Also returns an "Add to LinkedIn" certification deep link. LinkedIn does not
// ingest verifiable credentials: the link opens its certification form,
// pre-filled, whose "Show credential" URL resolves to the subject's PUBLIC verify
// proof. The signed credential is what carries the claims.
//
// Reuses the subject's EXISTING share token, so issuing never rotates a token or
// kills a badge they already published. Refuses test-mode subjects.
func (c *Client) MintProfessionalRecordCredential(ctx context.Context, userID string, ttlSeconds *int) (*ProfessionalRecordCredentialResult, error) {
	body := map[string]any{}
	if ttlSeconds != nil {
		body["ttlSeconds"] = *ttlSeconds
	}
	var out ProfessionalRecordCredentialResult
	if err := c.post(ctx, "/users/"+esc(userID)+"/professional-record/credential", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Career export + outcome templates ──────────────────────────────────────

// CareerExportDocument is a JSON Resume document (jsonresume.org) — the standard
// résumé sections plus a meta.credda block (reliability summary, verification
// depth, tenure, disclosures); every work/education/skills/certificates item
// carries a credda extension flagging verified vs self-reported and a per-item
// proof URL. Typed openly (a plain map) because it is a standard résumé document,
// not a Credda-shaped payload. It describes a record — never a hiring verdict.
type CareerExportDocument = map[string]any

// OutcomeTemplatesCatalog is the industry outcome-template catalog — for each
// industry, the concrete outcomes that matter, the ingest event type to report
// each as, a suggested stake, and (the load-bearing part) WHO the third-party
// witness is. Public, versioned, machine-readable guidance — same family as the
// plan and webhook-event catalogs. Typed openly. Guidance only: nothing here
// scores, writes, or ranks anyone.
type OutcomeTemplatesCatalog = map[string]any

// GetCareerExport fetches the subject's whole verified record as an OPEN JSON
// Resume document, so it drops into an ATS/HRIS or résumé tool without a bespoke
// Credda integration. GET /api/v1/users/:id/career-export (requires scores:read).
// Every item is flagged verified vs self-reported and verified items anchor to
// the subject's public proof URL.
//
// It describes a record — never a hiring verdict, a background check, or a
// consumer report.
func (c *Client) GetCareerExport(ctx context.Context, userID string) (CareerExportDocument, error) {
	var out CareerExportDocument
	if err := c.get(ctx, "/users/"+esc(userID)+"/career-export", true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPublicCareerExport fetches the subject's career export behind a public share
// token — the same JSON Resume document, consented via the token.
// GET /api/v1/verify/:token/career-export. No API key: the token is the subject's
// own consent to present it. Verified items anchor to this same public proof URL.
func (c *Client) GetPublicCareerExport(ctx context.Context, token string) (CareerExportDocument, error) {
	var out CareerExportDocument
	if err := c.get(ctx, "/verify/"+esc(token)+"/career-export", false, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetOutcomeTemplates fetches the industry outcome-template catalog. Pass an empty
// industry for the whole catalog, or a slug to filter to one set.
// GET /api/v1/outcome-templates. No API key required.
//
// Guidance only: a template never sets isVerified — only a genuine third-party
// witness confirming the outcome does.
func (c *Client) GetOutcomeTemplates(ctx context.Context, industry string) (OutcomeTemplatesCatalog, error) {
	qs := url.Values{}
	setStr(qs, "industry", industry)
	var out OutcomeTemplatesCatalog
	if err := c.get(ctx, withQuery("/outcome-templates", qs), false, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Quota transparency (GET /api/v1/usage/quota) ───────────────────────────

// QuotaPayload is the read-only monthly-quota state: how many calls remain and
// when the window resets. Unlimited is true when the tier has no quota configured.
type QuotaPayload struct {
	Platform          map[string]any `json:"platform"`
	RateLimitPerMin   int            `json:"rateLimitPerMin"`
	Unlimited         bool           `json:"unlimited"`
	Cap               *int           `json:"cap"`
	Used              int            `json:"used"`
	Remaining         *int           `json:"remaining"`
	UsedRatio         *float64       `json:"usedRatio"`
	ResetAt           string         `json:"resetAt"`
	SecondsUntilReset int            `json:"secondsUntilReset"`
}

// GetQuota reports how many calls you have left this month and when it resets,
// without pulling the full per-day usage breakdown. Read-only: it never counts
// against your quota, and mirrors the exact numbers the enforcement path uses,
// so you can self-throttle before ever seeing a 429. GET /api/v1/usage/quota.
func (c *Client) GetQuota(ctx context.Context) (*QuotaPayload, error) {
	var out QuotaPayload
	if err := c.get(ctx, "/usage/quota", true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Document advisory (POST /api/v1/users/:id/documents/analyze) ───────────

// DocumentClaimInput is one structured claim to advise on. Only EventType is
// required. CounterpartyRef's PRESENCE decides whether the claim can count as
// verified; its value is opaque.
type DocumentClaimInput struct {
	EventType        string   `json:"eventType"`
	Label            string   `json:"label,omitempty"`
	StakeLevel       string   `json:"stakeLevel,omitempty"`
	TransactionValue *float64 `json:"transactionValue,omitempty"`
	DaysLate         *int     `json:"daysLate,omitempty"`
	CounterpartyRef  string   `json:"counterpartyRef,omitempty"`
}

// DocumentClaimAdvice is the per-claim advice returned by AnalyzeDocument.
type DocumentClaimAdvice struct {
	EventType      string  `json:"eventType"`
	Label          *string `json:"label"`
	Polarity       string  `json:"polarity"`
	Witness        string  `json:"witness"`
	HasWitness     bool    `json:"hasWitness"`
	WillBeVerified bool    `json:"willBeVerified"`
	Recommendation string  `json:"recommendation"`
	Advice         string  `json:"advice"`
}

// DocumentAdvicePayload is the response from AnalyzeDocument. It writes nothing.
type DocumentAdvicePayload struct {
	UserID         string                `json:"userId"`
	Claims         []DocumentClaimAdvice `json:"claims"`
	Projection     map[string]any        `json:"projection"`
	WitnessGuide   map[string]any        `json:"witnessGuide"`
	Summary        string                `json:"summary"`
	FormulaVersion string                `json:"formulaVersion"`
	Note           string                `json:"note"`
}

// AnalyzeDocument advises, per structured claim, who the third-party witness is,
// whether it counts as verified as submitted, and (read-only) how adding the
// claims moves the score (as-submitted vs. if-all-confirmed). WRITES NOTHING — a
// claim is verified only when its witness confirms, never by this call.
// POST /api/v1/users/:id/documents/analyze.
func (c *Client) AnalyzeDocument(ctx context.Context, userID string, claims []DocumentClaimInput) (*DocumentAdvicePayload, error) {
	var out DocumentAdvicePayload
	body := map[string]any{"claims": claims}
	if err := c.post(ctx, "/users/"+esc(userID)+"/documents/analyze", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Dispatch reliability (GET /api/v1/users/:id/reliability?context=dispatch) ─

// DispatchFactor is one ranked driver in a dispatch reliability read. Direction
// is "adverse" or "supporting". A factor attributes evidence to the record; it
// never recommends an action.
type DispatchFactor struct {
	Code         string  `json:"code"`
	Direction    string  `json:"direction"`
	Title        string  `json:"title"`
	Contribution float64 `json:"contribution"`
}

// DispatchEvidence is the outcome tally a dispatch read rests on.
type DispatchEvidence struct {
	TotalOutcomes    int `json:"totalOutcomes"`
	VerifiedOutcomes int `json:"verifiedOutcomes"`
}

// DispatchReliabilityPayload is the compact pre-shift record read.
//
// Every pointer field is nil because the data genuinely does not exist (an
// unscored subject, an empty ledger) — never a placeholder zero. NoShowRate in
// particular: nil means "no outcomes recorded", which is NOT the same fact as a
// 0.0 no-show rate earned over a real record.
type DispatchReliabilityPayload struct {
	UserID                     string           `json:"userId"`
	Context                    string           `json:"context"`
	DispatchReliabilityVersion string           `json:"dispatchReliabilityVersion"`
	Score                      *float64         `json:"score"`
	Band                       *string          `json:"band"`
	Confidence                 *float64         `json:"confidence"`
	ScoreFrozen                bool             `json:"scoreFrozen"`
	FormulaVersion             *string          `json:"formulaVersion"`
	ComputedAt                 *string          `json:"computedAt"`
	Evidence                   DispatchEvidence `json:"evidence"`
	// NoShowRate is the breach-type share of the outcome record in [0,1]; nil
	// with no outcomes.
	NoShowRate         *float64         `json:"noShowRate"`
	OnTimeRate         *float64         `json:"onTimeRate"`
	DaysSinceLastEvent *int             `json:"daysSinceLastEvent"`
	TopFactors         []DispatchFactor `json:"topFactors"`
	Note               string           `json:"note"`
	Disclosures        []string         `json:"disclosures"`
}

// GetDispatchReliability returns reliability at dispatch — the compact record
// read to make before assigning a shift: score/band/confidence, verified-evidence
// counts, noShowRate, the on-time component, recency and the top ranked drivers,
// in a sub-1KB payload. GET /api/v1/users/:id/reliability?context=dispatch.
//
// Read-only: it projects the score the engine already computed and counts over
// the append-only ledger — it never computes or writes a score, and a subject
// that has never been scored reads nil rather than triggering a computation.
//
// EVIDENCE, NOT A VERDICT. No field says call / do-not-call or fit / unfit; you
// apply your own criteria and own the decision. If you use this read to SELECT
// workers, FCRA (or a local equivalent) may attach to that decision — scope it
// with your counsel.
func (c *Client) GetDispatchReliability(ctx context.Context, userID string) (*DispatchReliabilityPayload, error) {
	qs := url.Values{}
	qs.Set("context", "dispatch")
	var out DispatchReliabilityPayload
	if err := c.get(ctx, withQuery("/users/"+esc(userID)+"/reliability", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Usage meters (GET /api/v1/usage/meters) ────────────────────────────────

// UsageMeterRow is one metered dimension. Dimension is "total", "status_class"
// or "endpoint"; Value is that dimension's key (e.g. a route pattern).
type UsageMeterRow struct {
	Meter     string `json:"meter"`
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
	Quantity  int    `json:"quantity"`
}

// UsageMetersPayload is usage in a metered-billing (Stripe Billing Meters) shape.
// A pure reprojection of the SAME usage counters GetUsage reads: Credda emits
// usage, your biller prices it — no score, no money. From/To are the inclusive
// window bounds (ISO dates, YYYY-MM-DD).
type UsageMetersPayload struct {
	Platform struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Tier string `json:"tier"`
	} `json:"platform"`
	Window map[string]any  `json:"window"`
	From   string          `json:"from"`
	To     string          `json:"to"`
	Meters []UsageMeterRow `json:"meters"`
}

// AnalyticsWindow is the window controls shared by GetUsageMeters and the two
// analytics reads: a trailing Days window OR an explicit inclusive From/To range
// (ISO dates, YYYY-MM-DD). They are mutually exclusive server-side, and ranges
// are clamped to the server's retention. A nil *AnalyticsWindow sends no params
// and takes the server default.
type AnalyticsWindow struct {
	Days *int
	From string
	To   string
}

func analyticsQuery(w *AnalyticsWindow) url.Values {
	qs := url.Values{}
	if w == nil {
		return qs
	}
	setInt(qs, "days", w.Days)
	setStr(qs, "from", w.From)
	setStr(qs, "to", w.To)
	return qs
}

// GetUsageMeters returns this platform's usage as metered-billing meters: per
// dimension (total / status class / route pattern) request totals over the
// window. Same window controls as GetUsage. Requires the `usage` scope. Returns
// parsed JSON; the CSV export (?format=csv) is a raw-HTTP use case.
// GET /api/v1/usage/meters.
func (c *Client) GetUsageMeters(ctx context.Context, window *AnalyticsWindow) (*UsageMetersPayload, error) {
	var out UsageMetersPayload
	if err := c.get(ctx, withQuery("/usage/meters", analyticsQuery(window)), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Platform analytics (GET /api/v1/analytics/{events,scores}) ─────────────

// EventAnalyticsBucket is one day (Date set) or one event type (EventType set)
// of event volume. VerifiedShare/ConfirmedShare are nil when the bucket has no
// events — never a placeholder 0.
//
// Verified counts the raw isVerified flag, which on a directly-reported event is
// the reporting platform's OWN assertion. Confirmed counts only events a DISTINCT
// counterparty wrote by acting on a one-time confirmation token — the third-party
// evidence density of an integration. Confirmed is always a subset of Verified.
type EventAnalyticsBucket struct {
	Date           string   `json:"date,omitempty"`
	EventType      string   `json:"eventType,omitempty"`
	Total          int      `json:"total"`
	Verified       int      `json:"verified"`
	Confirmed      int      `json:"confirmed"`
	VerifiedShare  *float64 `json:"verifiedShare"`
	ConfirmedShare *float64 `json:"confirmedShare"`
}

// EventAnalyticsPayload is event volume over YOUR OWN ledger: by day (gaps
// filled) + by type + the verified and counterparty-confirmed shares.
// Aggregate-only — no subject identifiers appear anywhere in it.
type EventAnalyticsPayload struct {
	Window map[string]any `json:"window"`
	Totals struct {
		Total          int      `json:"total"`
		Verified       int      `json:"verified"`
		Confirmed      int      `json:"confirmed"`
		VerifiedShare  *float64 `json:"verifiedShare"`
		ConfirmedShare *float64 `json:"confirmedShare"`
	} `json:"totals"`
	Daily  []EventAnalyticsBucket `json:"daily"`
	ByType []EventAnalyticsBucket `json:"byType"`
}

// ScoreAnalyticsBand is one band's share of the caller's scored subjects.
type ScoreAnalyticsBand struct {
	Band     string   `json:"band"`
	MinScore float64  `json:"minScore"`
	Count    int      `json:"count"`
	Share    *float64 `json:"share"`
}

// ScoreAnalyticsPayload is the band mix, central tendency and movement over the
// caller's subjects. Median/Mean are nil on an empty book — not 0. Aggregate-only
// and read-only: a distribution fact about your book, never a verdict on any
// subject, and it never moves a score.
type ScoreAnalyticsPayload struct {
	FormulaVersion string         `json:"formulaVersion"`
	Window         map[string]any `json:"window"`
	ScoredSubjects int            `json:"scoredSubjects"`
	Central        struct {
		Median *float64 `json:"median"`
		Mean   *float64 `json:"mean"`
	} `json:"central"`
	BandDistribution []ScoreAnalyticsBand `json:"bandDistribution"`
	Movement         struct {
		Up                 int `json:"up"`
		Down               int `json:"down"`
		Unchanged          int `json:"unchanged"`
		SubjectsMoved      int `json:"subjectsMoved"`
		SubjectsRecomputed int `json:"subjectsRecomputed"`
	} `json:"movement"`
}

// GetEventAnalytics returns event analytics over YOUR OWN ledger: volume by day
// (gaps filled) + by type + the verified share, over a trailing days window
// (default 30, max 365) or an explicit from/to range. Aggregate-only — no subject
// identifiers. Requires scores:read; test/live isolated.
// GET /api/v1/analytics/events. Reports the verified AND counterparty-confirmed
// shares side by side — see EventAnalyticsBucket for why they are not the same.
func (c *Client) GetEventAnalytics(ctx context.Context, window *AnalyticsWindow) (*EventAnalyticsPayload, error) {
	var out EventAnalyticsPayload
	if err := c.get(ctx, withQuery("/analytics/events", analyticsQuery(window)), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetScoreAnalytics returns score analytics over YOUR subjects: band
// distribution, median/mean of current scores, and how many scores moved over the
// window. Same window controls as GetEventAnalytics. Aggregate-only; requires
// scores:read; test/live isolated. Read-only — never moves a score.
// GET /api/v1/analytics/scores.
func (c *Client) GetScoreAnalytics(ctx context.Context, window *AnalyticsWindow) (*ScoreAnalyticsPayload, error) {
	var out ScoreAnalyticsPayload
	if err := c.get(ctx, withQuery("/analytics/scores", analyticsQuery(window)), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ─── Activation campaigns (POST/GET /api/v1/activation/campaigns) ───────────

// ActivationRow is one roster row for CreateActivationCampaign — exactly a
// CreateConfirmationInput (embedded, so its fields serialise inline) plus an
// optional RowKey: your own stable id for the roster line (e.g. a shift id),
// which makes the campaign idempotent per row.
type ActivationRow struct {
	CreateConfirmationInput
	RowKey string `json:"rowKey,omitempty"`
}

// CreateActivationCampaignInput is the body for CreateActivationCampaign. Name
// is a display-only label ("March 2026 roster import") and is never scored; Rows
// is capped at 500 server-side (an over-cap submission is a 400, never truncated).
type CreateActivationCampaignInput struct {
	Name string          `json:"name,omitempty"`
	Rows []ActivationRow `json:"rows"`
}

// ActivationCampaignSummary is the campaign record itself — a label over a set of
// confirmation requests.
type ActivationCampaignSummary struct {
	ID             string  `json:"id"`
	Name           *string `json:"name"`
	SubmittedCount int     `json:"submittedCount"`
	CreatedAt      string  `json:"createdAt"`
}

// ActivationCampaignRowResult is one row's outcome — partial success. An ok row
// carries its one-time ConfirmationToken + hosted ConfirmURL; a failed one
// carries Error + Code. No row writes anything to the ledger.
type ActivationCampaignRowResult struct {
	Index             int    `json:"index"`
	OK                bool   `json:"ok"`
	UserID            string `json:"userId"`
	ID                string `json:"id,omitempty"`
	Status            string `json:"status,omitempty"`
	RowKey            string `json:"rowKey,omitempty"`
	ConfirmationToken string `json:"confirmationToken,omitempty"`
	ConfirmURL        string `json:"confirmUrl,omitempty"`
	Error             string `json:"error,omitempty"`
	Code              string `json:"code,omitempty"`
}

// ActivationCampaignDuplicate is one row dropped as an in-batch repeat of an
// earlier RowKey — no second token is ever minted for it.
type ActivationCampaignDuplicate struct {
	Index  int    `json:"index"`
	RowKey string `json:"rowKey"`
}

// ActivationCampaignResult is the result of CreateActivationCampaign.
type ActivationCampaignResult struct {
	Campaign   ActivationCampaignSummary     `json:"campaign"`
	Created    int                           `json:"created"`
	Failed     int                           `json:"failed"`
	Duplicates []ActivationCampaignDuplicate `json:"duplicates"`
	Results    []ActivationCampaignRowResult `json:"results"`
}

// ActivationFunnel is the factual counts a campaign reports — never a score or a
// judgement. ConfirmationRate is confirmed/submitted (0 when nothing submitted).
type ActivationFunnel struct {
	Submitted        int     `json:"submitted"`
	Pending          int     `json:"pending"`
	Confirmed        int     `json:"confirmed"`
	Declined         int     `json:"declined"`
	Expired          int     `json:"expired"`
	Cancelled        int     `json:"cancelled"`
	ConfirmationRate float64 `json:"confirmationRate"`
}

// ActivationCampaignFunnelPayload is the funnel GetActivationCampaign reports.
type ActivationCampaignFunnelPayload struct {
	Campaign ActivationCampaignSummary `json:"campaign"`
	Funnel   ActivationFunnel          `json:"funnel"`
}

// CreateActivationCampaign is the ACTIVATION ENGINE at book scale. Submit your
// whole historical roster/timesheets (up to 500 rows) in ONE call. Each row
// becomes an UNCONFIRMED confirmation request — a proposed outcome plus a
// one-time token — fanned out to its named counterparty, then
// GetActivationCampaign reports the funnel as those tokens are acted on.
// POST /api/v1/activation/campaigns.
//
// INVARIANT: a campaign writes NOTHING to the ledger. Every row flows through the
// SAME create path a single confirmation uses, so isVerified is still earned only
// on a genuine counterparty confirm — never here.
//
// Partial success: each Results entry carries a one-time token + hosted confirmUrl
// (ok rows) or an Error + Code (failed rows); in-batch duplicate RowKeys are
// dropped into Duplicates. If NOT ONE row could be created the call is a 400
// ACTIVATION_NO_VALID_ROWS. Give each row a RowKey to make the campaign idempotent
// per row; pass a stable idempotencyKey (empty string to omit) to make a retried
// whole submission exactly-once. Requires events:write.
func (c *Client) CreateActivationCampaign(ctx context.Context, input CreateActivationCampaignInput, idempotencyKey string) (*ActivationCampaignResult, error) {
	var headers map[string]string
	if idempotencyKey != "" {
		headers = map[string]string{"Idempotency-Key": idempotencyKey}
	}
	var out ActivationCampaignResult
	if err := c.post(ctx, "/activation/campaigns", input, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetActivationCampaign reports a campaign's funnel, derived LIVE from its
// confirmation requests, so it always reflects the current state as counterparties
// act on their tokens: submitted → confirmed / declined / pending (+ expired /
// cancelled), with a factual confirmationRate. Scoped to your platform + key mode.
// Requires events:write. GET /api/v1/activation/campaigns/{id}.
func (c *Client) GetActivationCampaign(ctx context.Context, id string) (*ActivationCampaignFunnelPayload, error) {
	var out ActivationCampaignFunnelPayload
	if err := c.get(ctx, "/activation/campaigns/"+esc(id), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
