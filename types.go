package credda

import "encoding/json"

// ─── Public / discovery ──────────────────────────────────────────────────────

// Plan is one developer tier from GET /api/v1/plans.
type Plan struct {
	ID              string   `json:"id"` // STARTER | GROWTH | ENTERPRISE
	Name            string   `json:"name"`
	Tagline         string   `json:"tagline"`
	Scopes          []string `json:"scopes"`
	RateLimitPerMin int      `json:"rateLimitPerMin"`
	// MonitorLimit is the cap on active continuous score monitors for this tier.
	MonitorLimit int `json:"monitorLimit"`
	// PriceUsdMonthly is the official monthly price in USD (display-only until
	// self-serve checkout is live).
	PriceUsdMonthly int      `json:"priceUsdMonthly"`
	Support         string   `json:"support"`
	Features        []string `json:"features"`
}

// PlanFeature is a feature row (label + group) for a comparison table.
type PlanFeature struct {
	Key   string `json:"key"`
	Group string `json:"group"`
	Label string `json:"label"`
}

// PlanCatalog is the developer plan catalog from GET /api/v1/plans — the tiers,
// their scopes, rate limits, official monthly prices and feature matrix.
type PlanCatalog struct {
	Pricing  string        `json:"pricing"` // "official"
	Note     string        `json:"note"`
	Features []PlanFeature `json:"features"`
	Plans    []Plan        `json:"plans"`
}

// ErrorCodeDoc is one documented error code from GET /api/v1/errors.
type ErrorCodeDoc struct {
	Code        string `json:"code"`
	HTTPStatus  int    `json:"httpStatus"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WhatToDo    string `json:"whatToDo"`
	// Retryable is true only when repeating the identical request can succeed
	// later without any change by the caller.
	Retryable bool `json:"retryable"`
}

// ErrorCatalog is the machine-readable error catalog from GET /api/v1/errors.
type ErrorCatalog struct {
	Envelope      map[string]string `json:"envelope"`
	RetryGuidance string            `json:"retryGuidance"`
	Tracing       string            `json:"tracing"`
	Codes         []ErrorCodeDoc    `json:"codes"`
}

// EnumValueDoc is one value of a documented enum. Value and Description are
// always present; Extra holds the enum-specific facts (weight for stakeLevel,
// minScore for scoreBand, trustMultiplier for platformTier, ingestible for
// eventType, terminal for disputeStatus).
type EnumValueDoc struct {
	Value       string         `json:"value"`
	Description string         `json:"description"`
	Extra       map[string]any `json:"-"`
}

// UnmarshalJSON keeps the known fields typed while preserving the per-enum
// extras, which differ by enum and would otherwise be silently dropped.
func (v *EnumValueDoc) UnmarshalJSON(data []byte) error {
	var all map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	if s, ok := all["value"].(string); ok {
		v.Value = s
	}
	if s, ok := all["description"].(string); ok {
		v.Description = s
	}
	delete(all, "value")
	delete(all, "description")
	v.Extra = all
	return nil
}

// EnumDoc is one documented enum from GET /api/v1/enums.
type EnumDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// UsedIn lists where this enum appears on the wire.
	UsedIn []string       `json:"usedIn"`
	Values []EnumValueDoc `json:"values"`
}

// EnumCatalog is every closed value set the API accepts or returns
// (GET /api/v1/enums).
type EnumCatalog struct {
	Note  string    `json:"note"`
	Enums []EnumDoc `json:"enums"`
}

// Reason-code directions. Informational is neither adverse nor supporting: it
// states a fact about the DATA (no recorded outcomes, or a score not computed
// yet) rather than about the record's performance.
//
// NEVER draw a Regulation B statement of specific reasons from an informational
// code. It is the reason there is no attribution, not attribution.
const (
	ReasonDirectionAdverse       = "adverse"
	ReasonDirectionSupporting    = "supporting"
	ReasonDirectionInformational = "informational"
)

// Data states reported by ReasonCodeResult.DataState and
// ReliabilityReportMetrics.DataState.
const (
	DataStateOK                  = "ok"
	DataStateNoRecordedOutcomes  = "no_recorded_outcomes"
	DataStateScoreNotYetComputed = "score_not_yet_computed"
)

// ReasonCodeDoc is one documented reason code from GET /api/v1/reason-codes.
type ReasonCodeDoc struct {
	Code   string `json:"code"`
	Factor string `json:"factor"`
	// Direction is one of the ReasonDirection* constants. The catalog serves
	// "informational" today for NO_RECORDED_OUTCOMES and SCORE_NOT_YET_COMPUTED.
	// filter those out of any adverse-action notice.
	Direction   string `json:"direction"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ReasonCodeInstance is one ranked reason-code instance for a specific subject,
// returned inside GET /score/explain under reasonCodes.
type ReasonCodeInstance struct {
	Code   string `json:"code"`
	Factor string `json:"factor"`
	// Direction is one of the ReasonDirection* constants.
	Direction   string `json:"direction"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Contribution is the importance-weighted contribution in [0,1], the ranking key.
	Contribution float64 `json:"contribution"`
	// Rank is 1-based within this instance's direction group.
	Rank int `json:"rank"`
	// Evidence holds the specific numbers behind the code, reproducible from the ledger.
	Evidence map[string]float64 `json:"evidence"`
}

// ReasonCodeResult is the reasonCodes object attached to GET /score/explain:
// deterministic factor attribution a partner draws its own Regulation B
// statement of specific reasons from. Credda supplies the attribution only; it
// takes no action and issues no notice.
//
// ⚠️ CHECK InsufficientData FIRST. When it is true, NOTHING is attributable:
// the record holds no outcomes at all, or it holds outcomes whose score has not
// been computed yet, and both ranked lists are EMPTY by construction. An absent
// measurement must never yield an adverse reason.
type ReasonCodeResult struct {
	FormulaVersion     string `json:"formulaVersion"`
	ReasonCodesVersion string `json:"reasonCodesVersion"`
	// FinalScore is nil when the subject has no computed score.
	FinalScore     *float64 `json:"finalScore"`
	Method         string   `json:"method"`
	KeyFactorLimit int      `json:"keyFactorLimit"`
	// AdverseActionReasons is ranked most-significant-first. Empty whenever
	// InsufficientData is true.
	AdverseActionReasons []ReasonCodeInstance `json:"adverseActionReasons"`
	// SupportingFactors is ranked. Empty whenever InsufficientData is true.
	SupportingFactors []ReasonCodeInstance `json:"supportingFactors"`
	// InformationalFactors are facts about the DATA rather than the record's
	// performance (NO_RECORDED_OUTCOMES, SCORE_NOT_YET_COMPUTED). Never draw a
	// statement of specific reasons from this list.
	InformationalFactors []ReasonCodeInstance `json:"informationalFactors"`
	// InsufficientData is true when nothing is attributable. Branch on it before
	// reading either ranked list or FinalScore.
	InsufficientData bool `json:"insufficientData"`
	// DataState is one of the DataState* constants and says WHICH kind of
	// unmeasured this is.
	DataState   string   `json:"dataState"`
	Disclosures []string `json:"disclosures"`
	Advisory    string   `json:"advisory"`
}

// ReasonCodeCatalog is the adverse-action reason-code catalog
// (GET /api/v1/reason-codes): the stable, versioned meaning of every reason
// code the scoring explanation can attribute. A B2B2C partner draws its
// Regulation B statement of specific reasons from a subject's ranked codes
// (returned on GET /score/explain). Credda supplies the attribution only — it
// is not a creditor and issues no decision or notice.
type ReasonCodeCatalog struct {
	ReasonCodesVersion string `json:"reasonCodesVersion"`
	FormulaVersion     string `json:"formulaVersion"`
	Note               string `json:"note"`
	Method             string `json:"method"`
	KeyFactorLimit     int    `json:"keyFactorLimit"`
	KeyFactorGuidance  string `json:"keyFactorGuidance"`
	// InsufficientDataPolicy states the rule for an UNMEASURED record: it yields
	// NO adverse reason, in either of the two ways a record can be unmeasured.
	InsufficientDataPolicy string          `json:"insufficientDataPolicy"`
	Disclosures            []string        `json:"disclosures"`
	Codes                  []ReasonCodeDoc `json:"codes"`
}

// WebhookEventDoc is one documented outbound event from GET /api/v1/webhooks/events.
type WebhookEventDoc struct {
	Type        string         `json:"type"` // one of the Webhook* event constants (score.*, dispute.resolved, monitor.triggered, usage.quota_warning)
	Description string         `json:"description"`
	Example     map[string]any `json:"example"`
}

// WebhookEventCatalog is the outbound webhook event catalog from
// GET /api/v1/webhooks/events: every event type the API can send, the delivery
// envelope, an example payload each, and signature-verification guidance.
type WebhookEventCatalog struct {
	Envelope   map[string]string `json:"envelope"`
	Signing    string            `json:"signing"`
	Advisory   string            `json:"advisory"`
	Events     []WebhookEventDoc `json:"events"`
	EventTypes []string          `json:"eventTypes"`
}

// TrustPayload is the public payload from GET /api/v1/verify/:token. It
// contains no platform user id.
// FinalScore and ScoreBand are POINTERS because a subject may have no computed
// score yet. They are nil in that case — never a placeholder. Decoding them as
// float64/string would turn a JSON null into 0/"", i.e. a score of zero.
type TrustPayload struct {
	Token             string   `json:"token"`
	FinalScore        *float64 `json:"finalScore"`
	ScoreBand         *string  `json:"scoreBand"`
	Confidence        float64  `json:"confidence"`
	VerifiedPlatforms int      `json:"verifiedPlatforms"`
	TotalEvents       int      `json:"totalEvents"`
	ScoreFrozen       bool     `json:"scoreFrozen"`
	FormulaVersion    string   `json:"formulaVersion"`
	ComputedAt        *string  `json:"computedAt"`
	Issuer            string   `json:"issuer"`

	// Credential is a signed, offline-verifiable Verifiable Trust Credential
	// (EdDSA JWT). Optional.
	Credential    string `json:"credential,omitempty"`
	CredentialKid string `json:"credentialKid,omitempty"`
	CredentialExp string `json:"credentialExp,omitempty"`
	JWKSURI       string `json:"jwksUri,omitempty"`
}

// VerificationMethod is one key entry in a DID document.
type VerificationMethod struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Controller   string         `json:"controller"`
	PublicKeyJWK map[string]any `json:"publicKeyJwk"`
}

// DIDService is one service endpoint in a DID document.
type DIDService struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// DIDDocument is the did:web document from GET /.well-known/did.json.
type DIDDocument struct {
	Context            []string             `json:"@context"`
	ID                 string               `json:"id"`
	VerificationMethod []VerificationMethod `json:"verificationMethod"`
	AssertionMethod    []string             `json:"assertionMethod"`
	Authentication     []string             `json:"authentication"`
	Service            []DIDService         `json:"service"`
}

// RegistryIssuer is a single issuer entry in the trust registry.
type RegistryIssuer struct {
	Name            string   `json:"name"`
	DID             string   `json:"did"`
	Status          string   `json:"status"`
	CredentialTypes []string `json:"credentialTypes"`
	DIDDocument     string   `json:"didDocument"`
	JWKSURI         string   `json:"jwksUri"`
}

// TrustRegistry comes from GET /.well-known/credda-trust-registry.json:
// Credda's own issuer entry plus any federated issuers it recognizes.
type TrustRegistry struct {
	Version string           `json:"version"`
	Issuers []RegistryIssuer `json:"issuers"`
}

// TrustExportScore is the plaintext score block of a trust export.
// FinalScore and ScoreBand are POINTERS because a subject may have no computed
// score yet. They are nil in that case — never a placeholder. Decoding them as
// float64/string would turn a JSON null into 0/"", i.e. a score of zero.
type TrustExportScore struct {
	FinalScore     *float64 `json:"finalScore"`
	ScoreBand      *string  `json:"scoreBand"`
	Confidence     float64  `json:"confidence"`
	FormulaVersion string   `json:"formulaVersion"`
	ComputedAt     *string  `json:"computedAt"`
	ScoreFrozen    bool     `json:"scoreFrozen"`
}

// TrustExportHistoryEntry is one historical snapshot in a trust export.
type TrustExportHistoryEntry struct {
	FinalScore float64 `json:"finalScore"`
	ScoreBand  string  `json:"scoreBand"`
	ComputedAt string  `json:"computedAt"`
}

// TrustExport is the portable, self-verifying bundle from
// GET /api/v1/verify/:token/export.
type TrustExport struct {
	Format     string `json:"format"` // "credda-trust-export/1"
	ExportedAt string `json:"exportedAt"`
	Subject    struct {
		Token string `json:"token"`
	} `json:"subject"`
	Score    TrustExportScore `json:"score"`
	Activity struct {
		VerifiedPlatforms int `json:"verifiedPlatforms"`
		TotalEvents       int `json:"totalEvents"`
	} `json:"activity"`
	History []TrustExportHistoryEntry `json:"history"`
	// Credential holds a signed W3C VC-JWT (offline-verifiable).
	Credential struct {
		Format string `json:"format"` // "jwt_vc_json"
		VC     string `json:"vc"`
		Issuer string `json:"issuer"`
	} `json:"credential"`
	Revocation struct {
		StatusListCredential string `json:"statusListCredential"`
	} `json:"revocation"`
	HowToVerify string `json:"howToVerify"`
}

// ─── Scores ──────────────────────────────────────────────────────────────────

// ScoreBreakdown is the raw factor breakdown behind a score.
type ScoreBreakdown struct {
	CR                      float64 `json:"cr"`
	OTR                     float64 `json:"otr"`
	DR                      float64 `json:"dr"`
	VD                      float64 `json:"vd"`
	PlatformTrustMultiplier float64 `json:"platformTrustMultiplier"`
	ConsistencyFactor       float64 `json:"consistencyFactor"`
	MomentumFactor          float64 `json:"momentumFactor"`
}

// ScorePayload comes from GET /api/v1/users/:id/score.
// FinalScore and ScoreBand are POINTERS because a subject may have no computed
// score yet. They are nil in that case — never a placeholder. Decoding them as
// float64/string would turn a JSON null into 0/"", i.e. a score of zero.
// Breakdown is likewise nil when there is no snapshot: a zeroed breakdown reads
// as a PERFECT record (dr 0 = no disputes, multipliers 1 = ideal).
type ScorePayload struct {
	UserID         string          `json:"userId"`
	FinalScore     *float64        `json:"finalScore"`
	ScoreBand      *string         `json:"scoreBand"`
	Confidence     float64         `json:"confidence"`
	Breakdown      *ScoreBreakdown `json:"breakdown"`
	FormulaVersion *string         `json:"formulaVersion"`
	VelocityFlag   bool            `json:"velocityFlag"`
	ComputedAt     *string         `json:"computedAt"`
	ScoreFrozen    *bool           `json:"scoreFrozen,omitempty"`
	FrozenAt       *string         `json:"frozenAt,omitempty"`
}

// BatchScoreEntry is one entry in a batch score read. Unknown ids come back
// with Error == "not_found" and no score fields set — check Error first.
type BatchScoreEntry struct {
	UserID      string   `json:"userId"`
	FinalScore  *float64 `json:"finalScore,omitempty"`
	ScoreBand   *string  `json:"scoreBand,omitempty"`
	ScoreFrozen *bool    `json:"scoreFrozen,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// BatchScoresPayload comes from POST /api/v1/users/scores.
type BatchScoresPayload struct {
	Scores         []BatchScoreEntry `json:"scores"`
	Count          int               `json:"count"`
	FormulaVersion string            `json:"formulaVersion"`
}

// ScoreExplainFactor is one factor in a plain-language score explanation.
//
// ⚠️ UNMEASURED IS NOT ZERO. Branch on Available before reading Value or
// Contribution: the API sends JSON null for both whenever the record holds no
// outcomes, and encoding/json silently leaves a non-pointer float64 at 0.0 in
// that case. A 0.0 here would read as "completed none of them" about a record
// that was never measured.
type ScoreExplainFactor struct {
	// Key is the stable machine key for this row: "completionRate",
	// "onTimeRate", "disputeRate", "verificationDepth",
	// "verifiedProfessionalGrounding". Match on this, never on Name: the human
	// label has been renamed before and will be again.
	Key  string `json:"key"`
	Name string `json:"name"`
	// Value is the factor value in [0,1]. VALID ONLY WHEN Available IS TRUE.
	Value float64 `json:"value"`
	// Weight is the fraction of the score this factor carries, in [0,1], e.g.
	// 0.37. It is DERIVED from GET /api/v1/scoring/model, so it is the weight
	// the engine actually applied.
	//
	// This was declared string until 2026-08-10, and that was not a narrow type,
	// it was the wrong one: the API split the field on 2026-08-09 into a numeric
	// weight and a separate WeightPercent label, and encoding/json refuses a
	// number into a string with a hard UnmarshalTypeError rather than a zero
	// value. GetScoreExplain therefore returned (nil, error) against the live
	// API for every caller, so no Go consumer can have been decoding this
	// successfully and no Go consumer can be broken by the correction.
	Weight float64 `json:"weight"`
	// WeightPercent is the same weight rendered for display, e.g. "37%". Use it
	// instead of formatting Weight yourself, so a change to how the API rounds
	// does not have to be mirrored here.
	WeightPercent string `json:"weightPercent"`
	// Contribution is weight × value. VALID ONLY WHEN Available IS TRUE.
	Contribution float64 `json:"contribution"`
	// Available is false when the record has no data from which to measure this
	// factor. This is the field to branch on.
	Available   bool   `json:"available"`
	Description string `json:"description"`
}

// ScoreExplainPayload comes from GET /api/v1/users/:id/score/explain.
type ScoreExplainPayload struct {
	Summary string               `json:"summary"`
	Factors []ScoreExplainFactor `json:"factors"`
	// DataSufficiency is the explicit insufficient-data state for the whole
	// explanation. Render it when InsufficientData is true instead of reading a
	// rate off an unmeasured record. Present on every response, including the
	// empty-record one (where Factors is empty).
	DataSufficiency *DataSufficiency `json:"dataSufficiency,omitempty"`
	// ReasonCodes is the deterministic adverse-action attribution (ECOA / Reg B)
	// for this record. Check ReasonCodes.InsufficientData before drawing a
	// statement of specific reasons from it.
	ReasonCodes   *ReasonCodeResult `json:"reasonCodes,omitempty"`
	PlatformTrust *struct {
		Explanation string  `json:"explanation"`
		AppliedTier string  `json:"appliedTier"`
		Multiplier  float64 `json:"multiplier"`
	} `json:"platformTrust,omitempty"`
	Consistency *struct {
		Factor      float64 `json:"factor"`
		Description string  `json:"description"`
	} `json:"consistency,omitempty"`
	Momentum *struct {
		Factor      float64 `json:"factor"`
		Direction   string  `json:"direction"`
		Description string  `json:"description"`
	} `json:"momentum,omitempty"`
	Confidence struct {
		EventsRecorded      int    `json:"eventsRecorded"`
		EventsNeededForFull int    `json:"eventsNeededForFull"`
		Level               string `json:"level"`
	} `json:"confidence"`
	RecencyWarning *string `json:"recencyWarning,omitempty"`
	ComputedAt     string  `json:"computedAt,omitempty"`
}

// ScoreHistoryPayload comes from GET /api/v1/users/:id/score/history. The
// snapshot rows are left as raw maps, matching the TS SDK's
// `Array<Record<string, unknown>>`.
type ScoreHistoryPayload struct {
	Data  []map[string]any `json:"data"`
	Count int              `json:"count"`
	// NextCursor: pass as Cursor to fetch the next page; nil once exhausted.
	NextCursor *string `json:"nextCursor,omitempty"`
}

// ScoreHistoryQuery are the optional filters for GetScoreHistory.
type ScoreHistoryQuery struct {
	From   string // ISO-8601
	To     string // ISO-8601
	Limit  *int
	Cursor string
}

// FactorDelta is one factor's movement between two score computations.
// Factor is one of "CR", "OTR", "DR", "VD".
type FactorDelta struct {
	Factor   string  `json:"factor"`
	Before   float64 `json:"before"`
	After    float64 `json:"after"`
	Delta    float64 `json:"delta"`
	Improved bool    `json:"improved"`
}

// ScoreSnapshotRef is a minimal reference to one score computation.
type ScoreSnapshotRef struct {
	FinalScore float64 `json:"finalScore"`
	ComputedAt string  `json:"computedAt"`
}

// ScoreDeltaPayload comes from GET /api/v1/users/:id/score/delta. Available is
// false until at least two computations exist.
type ScoreDeltaPayload struct {
	UserID          string            `json:"userId"`
	Available       bool              `json:"available"`
	From            *ScoreSnapshotRef `json:"from,omitempty"`
	To              *ScoreSnapshotRef `json:"to,omitempty"`
	ScoreDelta      *float64          `json:"scoreDelta,omitempty"`
	Direction       string            `json:"direction,omitempty"` // "up" | "down" | "unchanged"
	ConfidenceDelta *float64          `json:"confidenceDelta,omitempty"`
	MomentumDelta   *float64          `json:"momentumDelta,omitempty"`
	Factors         []FactorDelta     `json:"factors,omitempty"`
	TopDriver       *FactorDelta      `json:"topDriver,omitempty"`
	FormulaVersion  string            `json:"formulaVersion,omitempty"`
}

// ScoreComponent is one named, independently 0–100-scored component. Key is
// one of: reliability, timeliness, trustworthiness, verification, consistency,
// momentum.
//
// ⚠️ UNMEASURED IS NOT ZERO. Score is a POINTER, and nil means NOT MEASURED:
//
//	for _, c := range payload.Components {
//	    if c.Score == nil {
//	        fmt.Printf("%s: not measured\n", c.Label) // NOT "0"
//	        continue
//	    }
//	    fmt.Printf("%s: %.0f\n", c.Label, *c.Score)
//	}
//
// The API sends JSON null for Score whenever a component is not measurable
// (today: the record has no recorded outcomes, so every rate has a zero
// denominator). encoding/json decodes a null into a non-pointer float64 as a
// NO-OP (no error, field untouched), which is why this field is *float64: a
// plain float64 would read 0.0 in exactly that case, and in a product whose
// bands run down to At Risk a 0.0 is the worst possible record, not an unknown
// one. Available carries the same fact as a bool and is equally safe to branch
// on. A MEASURED zero is a non-nil pointer to 0 with Available: true and must
// still be displayed: hiding real bad news is a worse failure than the
// substitution this rule exists to prevent.
type ScoreComponent struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Score is 0–100, or nil when the component was not measured at all.
	// Available carries the same fact as a bool.
	Score *float64 `json:"score"`
	// Weight is the share of the weighted raw score this component drives, or nil
	// for the two multiplicative modifiers (consistency, momentum).
	Weight *float64 `json:"weight"`
	// Available is false when there is not enough data to measure this component
	// at all. False does NOT mean the component scored badly; it means there was
	// nothing to measure. This is the field to branch on.
	Available   bool   `json:"available"`
	Description string `json:"description"`
}

// DataSufficiency is the explicit insufficient-data state carried by
// GET /score/components and GET /score/explain.
//
// Render THIS when InsufficientData is true, rather than inventing "0%
// completion" for a record that has never been measured. No history is UNKNOWN,
// never BAD.
type DataSufficiency struct {
	// InsufficientData is true when the record holds NO outcome events at all.
	InsufficientData bool `json:"insufficientData"`
	// State is "ok" or the specific reason nothing can be measured
	// ("no_recorded_outcomes").
	State            string `json:"state"`
	RecordedOutcomes int    `json:"recordedOutcomes"`
	VerifiedOutcomes int    `json:"verifiedOutcomes"`
	// Note is plain-language copy that is safe to show verbatim.
	Note string `json:"note"`
}

// ScoreComponentsPayload comes from GET /api/v1/users/:id/score/components.
//
// The top-level Available reports whether a score has been COMPUTED at all; the
// per-component Available reports whether each component is MEASURABLE. They are
// different questions: a computed snapshot over an empty outcome ledger returns
// Available: true with every component Available: false.
type ScoreComponentsPayload struct {
	UserID     string           `json:"userId"`
	Available  bool             `json:"available"`
	FinalScore *float64         `json:"finalScore,omitempty"`
	ScoreBand  string           `json:"scoreBand,omitempty"`
	Components []ScoreComponent `json:"components"`
	// DataSufficiency is present on every response, including the empty-record
	// one (where Components is empty). Nil only when talking to an API older than
	// the field.
	DataSufficiency *DataSufficiency `json:"dataSufficiency,omitempty"`
	ComputedAt      string           `json:"computedAt,omitempty"`
	FormulaVersion  string           `json:"formulaVersion,omitempty"`
}

// ─── Verified Earnings ───────────────────────────────────────────────────────

// EarningsQuery selects the attestation window. From overrides Months.
type EarningsQuery struct {
	Months *int
	From   string
	To     string
}

// EarningsWindow is the resolved window an attestation covers.
type EarningsWindow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Months int    `json:"months"`
}

// EarningsPlatformTotal is one platform's contribution to an attested total.
type EarningsPlatformTotal struct {
	Platform   string  `json:"platform"`
	Gross      float64 `json:"gross"`
	EventCount int     `json:"eventCount"`
}

// EarningsPeriod is one UTC calendar month. Months with no earnings are present with 0.
type EarningsPeriod struct {
	Month             string                  `json:"month"`
	GrossVerified     float64                 `json:"grossVerified"`
	EventCount        int                     `json:"eventCount"`
	PlatformBreakdown []EarningsPlatformTotal `json:"platformBreakdown"`
}

// EarningsAttested holds the VERIFIED-only totals. Unverified value never appears here.
type EarningsAttested struct {
	GrossVerified     float64                 `json:"grossVerified"`
	EventCount        int                     `json:"eventCount"`
	Trailing12mTotal  float64                 `json:"trailing12mTotal"`
	PlatformCount     int                     `json:"platformCount"`
	PlatformBreakdown []EarningsPlatformTotal `json:"platformBreakdown"`
}

// EarningsStability holds the volatility/consistency metrics.
// CoefficientOfVariation is nil when there is no income to vary — never coerced to 0.
type EarningsStability struct {
	MonthsWithEarnings       int      `json:"monthsWithEarnings"`
	MedianMonthly            float64  `json:"medianMonthly"`
	MeanMonthly              float64  `json:"meanMonthly"`
	CoefficientOfVariation   *float64 `json:"coefficientOfVariation"`
	LongestConsecutiveMonths int      `json:"longestConsecutiveMonths"`
}

// EarningsUnverified is value that was REPORTED but not attested. It is never
// blended into any attested figure.
type EarningsUnverified struct {
	Gross      float64 `json:"gross"`
	EventCount int     `json:"eventCount"`
}

// EarningsExcluded records what was left out, so the omission is visible.
type EarningsExcluded struct {
	DisputedEvents  int     `json:"disputedEvents"`
	DisputedValue   float64 `json:"disputedValue"`
	ValuelessEvents int     `json:"valuelessEvents"`
}

// EarningsCoverage lets a consumer judge completeness of the record.
type EarningsCoverage struct {
	VerifiedShare     *float64 `json:"verifiedShare"`
	SelfReportedShare *float64 `json:"selfReportedShare"`
}

// VerifiedEarnings comes from GET /api/v1/users/:id/earnings — an attestation of
// income ALREADY RECORDED on the ledger. Currency is always null: amounts are
// platform-reported units. This is not an income verification for a credit
// decision and not a consumer report; see Disclosures.
type VerifiedEarnings struct {
	UserID             string             `json:"userId,omitempty"`
	EarningsVersion    string             `json:"earningsVersion"`
	Note               string             `json:"note"`
	Window             EarningsWindow     `json:"window"`
	Periods            []EarningsPeriod   `json:"periods"`
	Attested           EarningsAttested   `json:"attested"`
	Stability          EarningsStability  `json:"stability"`
	UnverifiedReported EarningsUnverified `json:"unverifiedReported"`
	Excluded           EarningsExcluded   `json:"excluded"`
	Coverage           EarningsCoverage   `json:"coverage"`
	Disclosures        []string           `json:"disclosures"`
}

// EarningsSummary comes from GET /api/v1/users/:id/earnings/summary.
type EarningsSummary struct {
	UserID                   string         `json:"userId,omitempty"`
	EarningsVersion          string         `json:"earningsVersion"`
	Note                     string         `json:"note"`
	Window                   EarningsWindow `json:"window"`
	Trailing12mVerifiedTotal float64        `json:"trailing12mVerifiedTotal"`
	MedianMonthly            float64        `json:"medianMonthly"`
	MonthsWithEarnings       int            `json:"monthsWithEarnings"`
	Volatility               *float64       `json:"volatility"`
	VerifiedShare            *float64       `json:"verifiedShare"`
	SelfReportedShare        *float64       `json:"selfReportedShare"`
	PlatformCount            int            `json:"platformCount"`
	LongestConsecutiveMonths int            `json:"longestConsecutiveMonths"`
	Disclosures              []string       `json:"disclosures"`
}

// EarningsCredentialResult comes from POST /api/v1/users/:id/earnings/credential —
// a signed, revocable W3C Verifiable Credential of type CreddaEarningsCredential.
type EarningsCredentialResult struct {
	Format          string         `json:"format"`
	CredentialVC    string         `json:"credentialVc"`
	CredentialType  string         `json:"credentialType"`
	Issuer          string         `json:"issuer"`
	Kid             string         `json:"kid"`
	Scope           string         `json:"scope"`
	EarningsVersion string         `json:"earningsVersion"`
	Claims          map[string]any `json:"claims"`
	IssuedAt        string         `json:"issuedAt"`
	ExpiresAt       string         `json:"expiresAt"`
	DIDDocument     string         `json:"didDocument"`
	TrustRegistry   string         `json:"trustRegistry"`
	StatusList      string         `json:"statusList"`
}

// ─── Timeline ────────────────────────────────────────────────────────────────

// TimelineDispute is the dispute marker on a timeline event item.
type TimelineDispute struct {
	Status     string  `json:"status"`
	ResolvedAt *string `json:"resolvedAt"`
}

// TimelineItem is one entry in a user's timeline. It is a flattened union of
// the TS SDK's two variants — check Type ("event" or "score_change") to know
// which fields are populated.
type TimelineItem struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	OccurredAt string `json:"occurredAt"`

	// Type == "event"
	EventType    string           `json:"eventType,omitempty"`
	PlatformName string           `json:"platformName,omitempty"`
	IsVerified   *bool            `json:"isVerified,omitempty"`
	StakeLevel   string           `json:"stakeLevel,omitempty"`
	DaysLate     *float64         `json:"daysLate,omitempty"`
	Dispute      *TimelineDispute `json:"dispute,omitempty"`

	// Type == "score_change"
	FinalScore *float64     `json:"finalScore,omitempty"`
	ScoreBand  string       `json:"scoreBand,omitempty"`
	ScoreDelta *float64     `json:"scoreDelta,omitempty"`
	Direction  *string      `json:"direction,omitempty"`
	TopDriver  *FactorDelta `json:"topDriver,omitempty"`
}

// TimelinePayload comes from GET /api/v1/users/:id/timeline.
type TimelinePayload struct {
	Data       []TimelineItem `json:"data"`
	Count      int            `json:"count"`
	NextCursor *string        `json:"nextCursor"`
}

// TimelineQuery are the optional pagination params for GetTimeline.
type TimelineQuery struct {
	Limit  *int
	Cursor string
}

// ─── Projection ──────────────────────────────────────────────────────────────

// ProjectionEventInput is one hypothetical prospective event for ProjectScore.
// Only EventType is required.
type ProjectionEventInput struct {
	EventType        string   `json:"eventType"`
	StakeLevel       string   `json:"stakeLevel,omitempty"`   // HIGH | MEDIUM | LOW
	PlatformTier     string   `json:"platformTier,omitempty"` // ENTERPRISE | GROWTH | STARTER | SELF_REPORTED
	IsVerified       *bool    `json:"isVerified,omitempty"`
	DaysLate         *float64 `json:"daysLate,omitempty"`
	TransactionValue *float64 `json:"transactionValue,omitempty"`
}

// ScoreBandRef pairs a score with its band.
type ScoreBandRef struct {
	FinalScore float64 `json:"finalScore"`
	ScoreBand  string  `json:"scoreBand"`
}

// TimelinessDisclosure reports what a batch of hypothetical claims did, and did
// not, state about lateness.
//
// An unstated lateness is NOT a claim of punctuality. The model gives an outcome
// with no recorded deadline full timeliness credit, so a projection over
// unstated claims is a BEST case: supplying the real lateness can only lower it.
// Branch on ProjectionIsUpperBound before presenting a projected number as a
// point estimate.
type TimelinessDisclosure struct {
	// StatedEvents counts the claims that stated a lateness. A stated 0 is a
	// claim of punctuality the caller made; an unstated one is a fact nobody
	// supplied. They are not interchangeable.
	StatedEvents   int `json:"statedEvents"`
	UnstatedEvents int `json:"unstatedEvents"`
	// Basis is "stated", "partially_stated" or "unstated".
	Basis string `json:"basis"`
	// ProjectionIsUpperBound is true when at least one claim left its lateness
	// unstated, which makes every projection in the response a best case.
	ProjectionIsUpperBound bool `json:"projectionIsUpperBound"`
	// Note states plainly what was and was not assumed. Safe to show verbatim.
	Note string `json:"note"`
	// ProjectedIfUnstatedWereLate is the other end of the range the request left
	// open: the same projection with every unstated lateness at the model's floor
	// point. Nil when nothing was left unstated. Present on ProjectScore only.
	ProjectedIfUnstatedWereLate *ProjectedBound `json:"projectedIfUnstatedWereLate,omitempty"`
}

// ProjectedBound is a projected score/band/delta at one end of a bounded range.
type ProjectedBound struct {
	FinalScore float64 `json:"finalScore"`
	ScoreBand  string  `json:"scoreBand"`
	Delta      float64 `json:"delta"`
}

// ScoreProjectionPayload comes from POST /api/v1/users/:id/score/project.
type ScoreProjectionPayload struct {
	UserID         string       `json:"userId"`
	Delta          float64      `json:"delta"`
	Current        ScoreBandRef `json:"current"`
	Projected      ScoreBandRef `json:"projected"`
	BandChanged    bool         `json:"bandChanged"`
	FormulaVersion string       `json:"formulaVersion"`
	// Timeliness says what the request stated about lateness, and whether the
	// projection above is an upper bound rather than a point estimate.
	Timeliness *TimelinessDisclosure `json:"timeliness,omitempty"`
}

// ─── Platforms / risk / usage ────────────────────────────────────────────────

// ContributingPlatform is one platform contributing to a user's score.
type ContributingPlatform struct {
	PlatformName       string `json:"platformName"`
	TrustTier          string `json:"trustTier"`
	EventCount         int    `json:"eventCount"`
	VerifiedEventCount int    `json:"verifiedEventCount"`
	CountsTowardVD     bool   `json:"countsTowardVD"`
}

// PlatformsPayload comes from GET /api/v1/users/:id/platforms.
type PlatformsPayload struct {
	Platforms []ContributingPlatform `json:"platforms"`
}

// RiskSignal is one advisory anti-gaming signal. Extra provider-specific
// fields are preserved in Extra.
type RiskSignal struct {
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
	Detail   string `json:"detail,omitempty"`
	// Extra holds the full raw signal object, since the API may attach
	// additional fields beyond the three above.
	Extra map[string]any `json:"-"`
}

// UnmarshalJSON keeps both the typed fields and the full raw object.
func (r *RiskSignal) UnmarshalJSON(b []byte) error {
	type alias RiskSignal
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*r = RiskSignal(a)
	raw := map[string]any{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	r.Extra = raw
	return nil
}

// RiskPayload comes from GET /api/v1/users/:id/risk. Advisory only — these
// signals never affect a score.
type RiskPayload struct {
	RiskLevel  string       `json:"riskLevel"`
	RiskScore  float64      `json:"riskScore"`
	Signals    []RiskSignal `json:"signals"`
	Advisory   bool         `json:"advisory"`
	ComputedAt string       `json:"computedAt"`
	// AISummary is an optional advisory AI narration; nil unless the AI
	// subsystem is enabled.
	AISummary json.RawMessage `json:"aiSummary,omitempty"`
}

// UsageDay is one day of API consumption, by status class.
type UsageDay struct {
	Date        string `json:"date"`
	Total       int    `json:"total"`
	OK          int    `json:"ok"`
	ClientError int    `json:"clientError"`
	ServerError int    `json:"serverError"`
}

// UsageTotals is UsageDay without the date.
type UsageTotals struct {
	Total       int `json:"total"`
	OK          int `json:"ok"`
	ClientError int `json:"clientError"`
	ServerError int `json:"serverError"`
}

// UsagePayload comes from GET /api/v1/usage — the calling platform's own
// consumption vs. its tier limits.
type UsagePayload struct {
	Platform struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Tier string `json:"tier"`
	} `json:"platform"`
	RateLimitPerMin int `json:"rateLimitPerMin"`
	Quota           struct {
		Cap       *int   `json:"cap"`
		Used      int    `json:"used"`
		Remaining *int   `json:"remaining"`
		ResetAt   string `json:"resetAt"`
	} `json:"quota"`
	// Window is the trailing-days window (Days) or an explicit range
	// (From/To, inclusive ISO dates, clamped to counter retention) —
	// whichever was requested; the other fields are zero-valued.
	Window struct {
		Days          int    `json:"days"`
		From          string `json:"from"`
		To            string `json:"to"`
		RequestedFrom string `json:"requestedFrom"`
		RequestedTo   string `json:"requestedTo"`
		Truncated     bool   `json:"truncated"`
		RetentionDays int    `json:"retentionDays"`
	} `json:"window"`
	Days   []UsageDay  `json:"days"`
	Totals UsageTotals `json:"totals"`
}

// ─── Activity log & own-event export ─────────────────────────────────────────

// ActivityEntry is one row of the platform's own activity log
// (GET /api/v1/activity).
type ActivityEntry struct {
	ID string `json:"id"`
	// Action is the audit action, e.g. EVENT_CREATED, WEBHOOK_UPDATED.
	Action string `json:"action"`
	// Payload is the action's recorded detail, as written — always includes
	// the calling platform's own platformId.
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"createdAt"`
}

// ActivityPayload is the cursor-paginated payload from GET /api/v1/activity
// (newest first).
type ActivityPayload struct {
	Data       []ActivityEntry `json:"data"`
	NextCursor *string         `json:"nextCursor"`
}

// ActivityQuery are the optional filters for GetActivity. From/To are ISO
// timestamps or dates bounding the row's createdAt.
type ActivityQuery struct {
	Limit  *int
	Cursor string
	Action string
	From   string
	To     string
}

// ExportedEvent is one exported event (GET /api/v1/events/export) — the
// platform's own recorded fields, keyed by its own external user id.
type ExportedEvent struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	EventType  string `json:"eventType"`
	StakeLevel string `json:"stakeLevel"`
	IsVerified bool   `json:"isVerified"`
	// AutoImported is a convenience view of metadata.autoImported == true.
	AutoImported     bool           `json:"autoImported"`
	TransactionValue *float64       `json:"transactionValue"`
	DueDate          *string        `json:"dueDate"`
	CompletedAt      *string        `json:"completedAt"`
	DaysLate         *int           `json:"daysLate"`
	CreatedAt        string         `json:"createdAt"`
	Metadata         map[string]any `json:"metadata"`
}

// EventExportPayload is the cursor-paginated payload from
// GET /api/v1/events/export (oldest first — ledger order).
type EventExportPayload struct {
	Data       []ExportedEvent `json:"data"`
	NextCursor *string         `json:"nextCursor"`
}

// EventExportQuery are the optional filters for ExportEvents. From/To are ISO
// timestamps or dates bounding the event's recorded createdAt.
type EventExportQuery struct {
	Limit  *int
	Cursor string
	From   string
	To     string
}

// ─── Event ingestion ─────────────────────────────────────────────────────────

// Event types accepted by POST /api/v1/events.
const (
	EventTransactionCompleted   = "TRANSACTION_COMPLETED"
	EventContractFulfilled      = "CONTRACT_FULFILLED"
	EventReviewVerified         = "REVIEW_VERIFIED"
	EventDisputeResolvedForUser = "DISPUTE_RESOLVED_FOR_USER"
	EventTransactionCancelled   = "TRANSACTION_CANCELLED"
	EventContractCancelled      = "CONTRACT_CANCELLED"
	EventContractBreached       = "CONTRACT_BREACHED"
	EventTransactionDisputed    = "TRANSACTION_DISPUTED"
)

// Additional event types the read-only what-if projection accepts
// (ProjectScore). The projection takes the FULL vocabulary — it writes nothing,
// so the dispute outcomes the API produces itself are modellable too, as is
// EventContractBreached above (the strongest negative signal in the formula).
// A platform cannot report these two via ReportEvent.
const (
	EventDisputeFiled               = "DISPUTE_FILED"
	EventDisputeResolvedAgainstUser = "DISPUTE_RESOLVED_AGAINST_USER"
)

// IngestEventTypes is every event type POST /api/v1/events accepts.
var IngestEventTypes = []string{
	EventTransactionCompleted,
	EventContractFulfilled,
	EventReviewVerified,
	EventDisputeResolvedForUser,
	EventTransactionCancelled,
	EventContractCancelled,
	EventContractBreached,
	EventTransactionDisputed,
}

// ProjectionEventTypes is every event type ProjectScore accepts — a superset of
// IngestEventTypes.
var ProjectionEventTypes = []string{
	EventTransactionCompleted,
	EventTransactionDisputed,
	EventTransactionCancelled,
	EventReviewVerified,
	EventContractFulfilled,
	EventContractCancelled,
	EventContractBreached,
	EventDisputeFiled,
	EventDisputeResolvedForUser,
	EventDisputeResolvedAgainstUser,
}

// Stake levels.
const (
	StakeHigh   = "HIGH"
	StakeMedium = "MEDIUM"
	StakeLow    = "LOW"
)

// ReportEventInput is the body for ReportEvent / POST /api/v1/events.
type ReportEventInput struct {
	UserID           string         `json:"userId"`
	EventType        string         `json:"eventType"`
	DueDate          string         `json:"dueDate,omitempty"`
	CompletedAt      string         `json:"completedAt,omitempty"`
	StakeLevel       string         `json:"stakeLevel,omitempty"`
	IsVerified       *bool          `json:"isVerified,omitempty"`
	TransactionValue *float64       `json:"transactionValue,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// BatchEventInput is one event in a batch (POST /api/v1/events/batch).
type BatchEventInput struct {
	ReportEventInput
	// IdempotencyKey (8–255 chars) makes a replayed item a no-op that returns
	// the original event id.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// ReportEventResult comes from POST /api/v1/events.
type ReportEventResult struct {
	Event   map[string]any `json:"event"`
	UserID  string         `json:"userId"`
	Dispute map[string]any `json:"dispute,omitempty"`
}

// BatchEventResultItem is one entry in a batch ingest result.
type BatchEventResultItem struct {
	Index   int    `json:"index"`
	UserID  string `json:"userId"`
	Status  string `json:"status"` // created | duplicate | failed
	EventID string `json:"eventId,omitempty"`
	Error   string `json:"error,omitempty"`
}

// BatchEventsResult comes from POST /api/v1/events/batch — partial success,
// one entry per input event.
type BatchEventsResult struct {
	Total     int                    `json:"total"`
	Created   int                    `json:"created"`
	Duplicate int                    `json:"duplicate"`
	Failed    int                    `json:"failed"`
	Results   []BatchEventResultItem `json:"results"`
}

// ─── Share tokens / disputes ─────────────────────────────────────────────────

// ShareTokenResult comes from POST /api/v1/users/:id/share-token.
type ShareTokenResult struct {
	Token        string `json:"token"`
	VerifyURL    string `json:"verifyUrl"`
	EmbedSnippet string `json:"embedSnippet"`
	WidgetSrc    string `json:"widgetSrc"`
}

// Dispute outcomes accepted by ResolveDispute.
const (
	DisputeForUser     = "FOR_USER"
	DisputeAgainstUser = "AGAINST_USER"
)

// DisputeResult comes from PATCH /api/v1/disputes/:id/resolve.
type DisputeResult struct {
	Dispute map[string]any `json:"dispute"`
}

// ─── Webhook management ──────────────────────────────────────────────────────

// Trust events a webhook can subscribe to.
const (
	WebhookScoreUpdated      = "score.updated"
	WebhookScoreBandChanged  = "score.band_changed"
	WebhookDisputeResolved   = "dispute.resolved"
	WebhookMonitorTriggered  = "monitor.triggered"
	WebhookUsageQuotaWarning = "usage.quota_warning"
)

// CreateWebhookInput is the body for CreateWebhook.
type CreateWebhookInput struct {
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	Description string   `json:"description,omitempty"`
}

// WebhookConfig is the public webhook projection (never includes the secret).
type WebhookConfig struct {
	ID             string   `json:"id"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
	Description    *string  `json:"description"`
	IsActive       bool     `json:"isActive"`
	FailureCount   int      `json:"failureCount"`
	DeliveredCount int      `json:"deliveredCount"`
	LastStatus     *int     `json:"lastStatus"`
	LastError      *string  `json:"lastError"`
	LastDeliveryAt *string  `json:"lastDeliveryAt"`
	DisabledAt     *string  `json:"disabledAt"`
	CreatedAt      string   `json:"createdAt"`
}

// CreateWebhookResult carries the new webhook plus its signing secret, which
// is shown ONCE — store it to verify deliveries.
type CreateWebhookResult struct {
	Webhook WebhookConfig `json:"webhook"`
	Secret  string        `json:"secret"`
}

// UpdateWebhookInput patches a webhook. Nil fields are omitted from the
// request body.
type UpdateWebhookInput struct {
	URL         *string  `json:"url,omitempty"`
	Events      []string `json:"events,omitempty"`
	Description *string  `json:"description,omitempty"`
	IsActive    *bool    `json:"isActive,omitempty"`
}

// WebhookTestResult comes from POST /api/v1/webhooks/:id/test.
type WebhookTestResult struct {
	Delivered  bool    `json:"delivered"`
	StatusCode *int    `json:"statusCode"`
	Error      *string `json:"error"`
	DurationMs *int    `json:"durationMs"`
}

// WebhookDelivery is one recorded delivery attempt.
type WebhookDelivery struct {
	ID         string  `json:"id"`
	WebhookID  string  `json:"webhookId"`
	EventID    string  `json:"eventId"`
	EventType  string  `json:"eventType"`
	Success    bool    `json:"success"`
	StatusCode *int    `json:"statusCode"`
	Error      *string `json:"error"`
	DurationMs *int    `json:"durationMs"`
	CreatedAt  string  `json:"createdAt"`
}

// WebhookListResult is the envelope returned by ListWebhooks.
type WebhookListResult struct {
	Data []WebhookConfig `json:"data"`
}

// WebhookUpdateResult is the envelope returned by UpdateWebhook.
type WebhookUpdateResult struct {
	Webhook WebhookConfig `json:"webhook"`
}

// WebhookDeliveriesResult is the cursor-paginated envelope returned by
// GetWebhookDeliveries.
type WebhookDeliveriesResult struct {
	Data       []WebhookDelivery `json:"data"`
	NextCursor *string           `json:"nextCursor"`
}

// RecentWebhookEventDelivery is the delivery attempt that carried an event. It
// is nil for catalog examples.
type RecentWebhookEventDelivery struct {
	ID          string `json:"id"`
	WebhookID   string `json:"webhookId"`
	Attempt     int    `json:"attempt"`
	Success     bool   `json:"success"`
	StatusCode  *int   `json:"statusCode"`
	DeliveredAt string `json:"deliveredAt"`
}

// RecentWebhookEvent is one item from GET /api/v1/webhooks/deliveries: the
// delivery envelope exactly as sent, plus provenance. IsExample is true for a
// representative payload from the event catalog — sample data, NOT a delivery
// that occurred. Never present an example as a real outcome.
type RecentWebhookEvent struct {
	// ID is the event id — stable across retries and replays. Dedupe on it.
	ID        string                      `json:"id"`
	Type      string                      `json:"type"`
	Livemode  bool                        `json:"livemode"`
	CreatedAt string                      `json:"createdAt"`
	Data      json.RawMessage             `json:"data"`
	IsExample bool                        `json:"isExample"`
	Delivery  *RecentWebhookEventDelivery `json:"delivery"`
}

// RecentWebhookEventsResult is the cursor-paginated envelope returned by
// GetRecentWebhookEvents. Source is "examples" when nothing has been delivered
// yet and the event catalog was used as sample data.
type RecentWebhookEventsResult struct {
	Data       []RecentWebhookEvent `json:"data"`
	NextCursor *string              `json:"nextCursor"`
	Source     string               `json:"source"`
}

// ─── Score monitors (continuous monitoring) ──────────────────────────────────

// ScoreMonitor is an edge-triggered threshold/band watch on one of your users.
// UserID is the platform's own externalId. Monitors are notification config
// only — a monitor never affects a score.
type ScoreMonitor struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	// Fires when the score crosses DOWN through this threshold.
	BelowScore *float64 `json:"belowScore"`
	// Fires when the score crosses UP through this threshold.
	AboveScore *float64 `json:"aboveScore"`
	// Fires whenever the score band label changes.
	OnBandChange    bool    `json:"onBandChange"`
	IsActive        bool    `json:"isActive"`
	LastTriggeredAt *string `json:"lastTriggeredAt"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

// CreateMonitorInput is the body for CreateMonitor — at least one condition
// (BelowScore, AboveScore, or OnBandChange) is required.
type CreateMonitorInput struct {
	UserID       string   `json:"userId"`
	BelowScore   *float64 `json:"belowScore,omitempty"`
	AboveScore   *float64 `json:"aboveScore,omitempty"`
	OnBandChange *bool    `json:"onBandChange,omitempty"`
}

// UpdateMonitorInput patches a monitor. Nil fields are omitted; to clear a
// threshold send an explicit JSON null via a custom body — or deactivate and
// recreate. The updated monitor must keep at least one condition.
type UpdateMonitorInput struct {
	BelowScore   *float64 `json:"belowScore,omitempty"`
	AboveScore   *float64 `json:"aboveScore,omitempty"`
	OnBandChange *bool    `json:"onBandChange,omitempty"`
	IsActive     *bool    `json:"isActive,omitempty"`
}

// MonitorResult is the envelope returned by CreateMonitor / GetMonitor /
// UpdateMonitor.
type MonitorResult struct {
	Monitor ScoreMonitor `json:"monitor"`
}

// MonitorListResult is the cursor-paginated envelope returned by ListMonitors.
type MonitorListResult struct {
	Data       []ScoreMonitor `json:"data"`
	NextCursor *string        `json:"nextCursor"`
}

// ─── Bulk screenings (async batch score reads) ───────────────────────────────

// ScreeningJob is one screening job's status + summary (results are fetched
// separately via GetScreeningResults). Status is one of QUEUED, RUNNING,
// COMPLETED, FAILED.
type ScreeningJob struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// Deduped ids submitted.
	TotalCount int `json:"totalCount"`
	// Ids that resolved to a known user. Nil until processed.
	FoundCount  *int    `json:"foundCount"`
	Error       *string `json:"error"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt"`
	CompletedAt *string `json:"completedAt"`
}

// ScreeningResultItem is one screened user. Score fields are non-nil only
// when Found is true.
type ScreeningResultItem struct {
	ExternalID string   `json:"externalId"`
	Found      bool     `json:"found"`
	Score      *float64 `json:"score,omitempty"`
	Band       *string  `json:"band,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	ComputedAt *string  `json:"computedAt,omitempty"`
}

// ScreeningResult is the envelope returned by CreateScreening / GetScreening.
type ScreeningResult struct {
	Screening ScreeningJob `json:"screening"`
}

// ScreeningListResult is the cursor-paginated envelope returned by
// ListScreenings.
type ScreeningListResult struct {
	Data       []ScreeningJob `json:"data"`
	NextCursor *string        `json:"nextCursor"`
}

// ScreeningResultsResult is the envelope returned by GetScreeningResults.
type ScreeningResultsResult struct {
	Screening ScreeningJob          `json:"screening"`
	Results   []ScreeningResultItem `json:"results"`
	Count     int                   `json:"count"`
}

// ─── Data ingress: field mapping + historical CSV import ─────────────────────
//
// A mapping is DECLARATIVE DATA, never code: a rule may read a dot-path,
// supply a constant, look a value up in your own table, or apply one of a
// fixed transform whitelist. There is no expression language, by design.

// Transform names accepted by a MappingFieldRule.
const (
	TransformCentsToUnits = "cents_to_units"
	TransformISODate      = "iso_date"
	TransformLowercase    = "lowercase"
	TransformTrim         = "trim"
	TransformBoolean      = "boolean"
)

// MappingFieldRule is the object form of a field rule. A bare dot-path string
// is also accepted on the wire — use MappingPath for that shorthand.
// Transform holds a single transform name or a []string of them, applied
// left-to-right BEFORE the Values lookup.
type MappingFieldRule struct {
	Path      string         `json:"path,omitempty"`
	Const     any            `json:"const,omitempty"`
	Values    map[string]any `json:"values,omitempty"`
	Default   any            `json:"default,omitempty"`
	Transform any            `json:"transform,omitempty"`
}

// MappingPath is the dot-path shorthand for a field rule (marshals to a bare
// JSON string, exactly like the object form with only Path set).
type MappingPath string

// IngestMapping declares how to reach Credda's event fields from YOUR record
// shape. Keys are Credda fields (userId, eventType, dueDate, completedAt,
// stakeLevel, isVerified, transactionValue, metadata, idempotencyKey) plus
// verifiedBy — the counterparty/witness identifier that licenses
// isVerified: true. Without it a record still ingests, with isVerified false
// and a warning. Values are a MappingPath or a MappingFieldRule.
type IngestMapping map[string]any

// IngestResultItem is one record's outcome. Records fail INDIVIDUALLY — a bad
// record never fails the rest. Status is one of created, duplicate, failed.
type IngestResultItem struct {
	Index   int    `json:"index"`
	UserID  string `json:"userId,omitempty"`
	Status  string `json:"status"`
	EventID string `json:"eventId,omitempty"`
	Error   string `json:"error,omitempty"`
	// Non-fatal notes — most commonly an isVerified downgrade.
	Warnings []string `json:"warnings,omitempty"`
}

// IngestResult is the envelope returned by Ingest.
type IngestResult struct {
	Total     int                `json:"total"`
	Created   int                `json:"created"`
	Duplicate int                `json:"duplicate"`
	Failed    int                `json:"failed"`
	Results   []IngestResultItem `json:"results"`
}

// StoredMapping is a saved, reusable mapping.
type StoredMapping struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description *string       `json:"description"`
	Mapping     IngestMapping `json:"mapping"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

// MappingResult is the envelope returned by CreateMapping / GetMapping.
type MappingResult struct {
	Mapping StoredMapping `json:"mapping"`
}

// MappingListResult is the cursor-paginated envelope from ListMappings.
type MappingListResult struct {
	Data       []StoredMapping `json:"data"`
	NextCursor *string         `json:"nextCursor"`
}

// ImportJob is one historical CSV import (status + counts). Status is one of
// QUEUED, RUNNING, COMPLETED, FAILED.
type ImportJob struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// Data rows in the file (never truncated — an over-cap file is refused).
	TotalRows    int `json:"totalRows"`
	CreatedCount int `json:"createdCount"`
	// Rows already present under their idempotency key (a safe re-upload).
	SkippedCount int `json:"skippedCount"`
	// Authoritative even when the stored error list is capped.
	FailedCount int     `json:"failedCount"`
	Error       *string `json:"error"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt"`
	CompletedAt *string `json:"completedAt"`
}

// ImportResult is the envelope returned by CreateImport / GetImport.
type ImportResult struct {
	Import ImportJob `json:"import"`
}

// ImportListResult is the cursor-paginated envelope from ListImports.
type ImportListResult struct {
	Data       []ImportJob `json:"data"`
	NextCursor *string     `json:"nextCursor"`
}

// ImportRowError is one rejected row. Row is 1-based over DATA rows (the
// header is excluded), so it lines up with a spreadsheet minus one.
type ImportRowError struct {
	Row    int    `json:"row"`
	Error  string `json:"error"`
	UserID string `json:"userId,omitempty"`
}

// ImportRowWarning is one non-fatal per-row note (e.g. an isVerified
// downgrade). The row was still imported.
type ImportRowWarning struct {
	Row     int    `json:"row"`
	Warning string `json:"warning"`
	UserID  string `json:"userId,omitempty"`
}

// ImportErrorsResult is the envelope returned by GetImportErrors.
type ImportErrorsResult struct {
	Import       ImportJob          `json:"import"`
	Errors       []ImportRowError   `json:"errors"`
	ErrorCount   int                `json:"errorCount"`
	Warnings     []ImportRowWarning `json:"warnings"`
	WarningCount int                `json:"warningCount"`
	// True when more rows failed than the stored list retains.
	Truncated bool `json:"truncated"`
}

// ─── Agent subjects + delivery receipts ──────────────────────────────────────

// AgentDeclaration holds caller-declared facts about an agent. Claims, never
// evidence, and never a scoring input.
type AgentDeclaration struct {
	OperatorName     string `json:"operatorName,omitempty"`
	OperatorHomepage string `json:"operatorHomepage,omitempty"`
	OperatorDid      string `json:"operatorDid,omitempty"`
	ModelFamily      string `json:"modelFamily,omitempty"`
	Description      string `json:"description,omitempty"`
	RegisteredBy     string `json:"registeredByPlatformId,omitempty"`
	RegisteredAt     string `json:"registeredAt,omitempty"`
	// OperatorIsRegisteredPlatform is true when the declared operator is an
	// identifiable Credda platform.
	OperatorIsRegisteredPlatform bool `json:"operatorIsRegisteredPlatform"`
}

// AgentOperatorInput names the party that operates an agent.
type AgentOperatorInput struct {
	Name     string `json:"name,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	Did      string `json:"did,omitempty"` // e.g. did:web:acme.ai
}

// RegisterAgentInput is the body of POST /api/v1/agents.
type RegisterAgentInput struct {
	UserID string `json:"userId"`
	// OperatedByReportingPlatform nil means the server default (true — the
	// conservative reading, under which your own reports are never confirmed
	// evidence for this agent).
	OperatedByReportingPlatform *bool `json:"operatedByReportingPlatform,omitempty"`
	// Operator must name a third-party operator when
	// OperatedByReportingPlatform is explicitly false.
	Operator    *AgentOperatorInput `json:"operator,omitempty"`
	ModelFamily string              `json:"modelFamily,omitempty"`
	Description string              `json:"description,omitempty"`
}

// AgentSubject is the result of registering an agent.
type AgentSubject struct {
	UserID      string           `json:"userId"`
	SubjectType string           `json:"subjectType"`
	Agent       AgentDeclaration `json:"agent"`
	CreatedAt   string           `json:"createdAt"`
	// SelfDealingRule states, in plain language, the rule just opted into.
	SelfDealingRule string `json:"selfDealingRule,omitempty"`
}

// DeliveryRecord tallies delivery outcomes from the append-only ledger, split by
// whether an independent counterparty was on the other side.
type DeliveryRecord struct {
	Deliveries int `json:"deliveries"`
	// ConfirmedDeliveries counts only outcomes a DISTINCT counterparty
	// attested — the only ones that are evidence.
	ConfirmedDeliveries   int `json:"confirmedDeliveries"`
	UnconfirmedDeliveries int `json:"unconfirmedDeliveries"`
	// SelfAttestedDeliveries were reported by the agent's own declared
	// operator, and are never confirmed evidence.
	SelfAttestedDeliveries int `json:"selfAttestedDeliveries"`
	Failures               int `json:"failures"`
	Disputes               int `json:"disputes"`
	// OnTimeConfirmedDeliveries and OnTimeRate are nil when nothing is
	// confirmed yet — an absent rate is not a perfect one.
	OnTimeConfirmedDeliveries *int     `json:"onTimeConfirmedDeliveries"`
	OnTimeRate                *float64 `json:"onTimeRate"`
	FirstRecordedAt           *string  `json:"firstRecordedAt"`
	LastRecordedAt            *string  `json:"lastRecordedAt"`
}

// DeliveryRecordDisclaimer states what a delivery record is — and what it is not.
type DeliveryRecordDisclaimer struct {
	IsA             string   `json:"isA"`
	IsNot           []string `json:"isNot"`
	SelfDealingRule string   `json:"selfDealingRule"`
}

// AgentScore is the subject's current deterministic score — the identical
// formula every subject runs through.
// FinalScore and ScoreBand are POINTERS because a subject may have no computed
// score yet. They are nil in that case — never a placeholder. Decoding them as
// float64/string would turn a JSON null into 0/"", i.e. a score of zero.
type AgentScore struct {
	FinalScore     *float64 `json:"finalScore"`
	ScoreBand      *string  `json:"scoreBand"`
	Confidence     float64  `json:"confidence"`
	FormulaVersion string   `json:"formulaVersion"`
	ComputedAt     string   `json:"computedAt"`
	ScoreFrozen    bool     `json:"scoreFrozen"`
}

// AgentDetail is GET /api/v1/agents/:id.
type AgentDetail struct {
	UserID         string                    `json:"userId"`
	SubjectType    string                    `json:"subjectType"`
	Agent          AgentDeclaration          `json:"agent"`
	CreatedAt      string                    `json:"createdAt"`
	Score          *AgentScore               `json:"score"`
	DeliveryRecord *DeliveryRecord           `json:"deliveryRecord"`
	Disclaimer     *DeliveryRecordDisclaimer `json:"disclaimer"`
}

// DeliveryReceipts is GET /api/v1/verify/:token/delivery-receipts.
type DeliveryReceipts struct {
	Token          string                   `json:"token"`
	SubjectType    string                   `json:"subjectType"`
	Agent          *AgentDeclaration        `json:"agent"`
	DeliveryRecord DeliveryRecord           `json:"deliveryRecord"`
	Score          AgentScore               `json:"score"`
	Disclaimer     DeliveryRecordDisclaimer `json:"disclaimer"`
	// CredentialVc is a signed W3C Verifiable Credential (VC-JWT) of the record.
	CredentialVc  string `json:"credentialVc"`
	Format        string `json:"format"`
	Issuer        string `json:"issuer"`
	Kid           string `json:"kid"`
	Scope         string `json:"scope"`
	IssuedAt      string `json:"issuedAt"`
	ExpiresAt     string `json:"expiresAt"`
	DidDocument   string `json:"didDocument"`
	TrustRegistry string `json:"trustRegistry"`
}
