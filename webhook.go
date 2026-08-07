package credda

// Verification of inbound Credda webhooks.
//
// Credda signs each delivery with `X-Credda-Signature: sha256=<hex>` where the
// HMAC-SHA256 is computed over `{X-Credda-Timestamp}.{rawBody}` using the
// webhook's signing secret. Verify on the RAW request body — before unmarshalling
// JSON — or the bytes won't match.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DefaultWebhookTolerance is the replay window applied when
// VerifyWebhookInput.Tolerance is nil, matching the TypeScript SDK.
const DefaultWebhookTolerance = 5 * time.Minute

// Verification failure sentinels. Use errors.Is to branch.
var (
	ErrMissingSignature  = errors.New("credda: missing signature header")
	ErrMissingTimestamp  = errors.New("credda: missing timestamp header")
	ErrInvalidTimestamp  = errors.New("credda: invalid timestamp")
	ErrTimestampSkew     = errors.New("credda: timestamp outside tolerance (possible replay)")
	ErrSignatureMismatch = errors.New("credda: signature mismatch")
	ErrInvalidBody       = errors.New("credda: webhook body is not valid JSON")
	ErrMissingFields     = errors.New("credda: webhook body is missing required fields (id, type, createdAt)")
)

// VerifyWebhookInput carries everything needed to verify one delivery.
type VerifyWebhookInput struct {
	// Secret is the webhook's signing secret (`whsec_…`).
	Secret string
	// RawBody is the exact raw request body bytes (verify BEFORE parsing JSON).
	RawBody string
	// SignatureHeader is the `X-Credda-Signature` value (`sha256=<hex>`).
	SignatureHeader string
	// TimestampHeader is the `X-Credda-Timestamp` value (unix seconds).
	TimestampHeader string
	// Tolerance rejects deliveries whose timestamp drifts more than this.
	// nil means DefaultWebhookTolerance; an explicit 0 disables the check.
	Tolerance *time.Duration
	// Now overrides the current time — for tests. Zero value means time.Now().
	Now time.Time
}

// Duration is a helper for setting VerifyWebhookInput.Tolerance.
func Duration(d time.Duration) *time.Duration { return &d }

// WebhookFactorDelta is one factor's movement between the prior and new score.
type WebhookFactorDelta = FactorDelta

// ScoreChangeReason explains why the score moved — a factor-level diff vs. the
// prior snapshot.
type ScoreChangeReason struct {
	ScoreDelta      float64              `json:"scoreDelta"`
	Direction       string               `json:"direction"` // up | down | unchanged
	Factors         []WebhookFactorDelta `json:"factors"`
	TopDriver       *WebhookFactorDelta  `json:"topDriver"`
	ConfidenceDelta float64              `json:"confidenceDelta"`
	MomentumDelta   float64              `json:"momentumDelta"`
}

// ScoreEventData is the payload of score.updated / score.band_changed.
type ScoreEventData struct {
	User struct {
		ExternalID string `json:"externalId"`
	} `json:"user"`
	Score          float64  `json:"score"`
	Band           string   `json:"band"`
	PreviousScore  *float64 `json:"previousScore"`
	PreviousBand   *string  `json:"previousBand"`
	Confidence     float64  `json:"confidence"`
	FormulaVersion string   `json:"formulaVersion"`
	ComputedAt     string   `json:"computedAt"`
	// Reason is nil on a user's first score.
	Reason *ScoreChangeReason `json:"reason,omitempty"`
}

// DisputeResolvedData is the payload of dispute.resolved.
type DisputeResolvedData struct {
	DisputeID string `json:"disputeId"`
	// EventID is present for platform-adjudicated resolutions; omitted for
	// auto-lapses.
	EventID string `json:"eventId,omitempty"`
	User    struct {
		ExternalID *string `json:"externalId"`
	} `json:"user"`
	Outcome string `json:"outcome"` // FOR_USER | AGAINST_USER
	Status  string `json:"status"`  // RESOLVED_FOR_USER | RESOLVED_AGAINST_USER
	// Lapsed is true when the dispute lapsed unadjudicated (resolved in the
	// user's favour).
	Lapsed     bool   `json:"lapsed"`
	ResolvedAt string `json:"resolvedAt"`
}

// WebhookEvent is a verified webhook event. Switch on Type, then call
// ScoreData or DisputeData to decode the matching payload.
type WebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Livemode is false when the event was produced by test-mode (crd_test_)
	// activity, true for live activity (Stripe convention). Deliveries recorded
	// before test mode existed omit the field; use IsLive to treat a missing
	// value as live.
	Livemode  *bool           `json:"livemode,omitempty"`
	CreatedAt string          `json:"createdAt"`
	Data      json.RawMessage `json:"data"`
}

// IsLive reports whether the event came from live activity. A missing
// livemode field (pre-test-mode deliveries) is treated as live.
func (e *WebhookEvent) IsLive() bool {
	return e.Livemode == nil || *e.Livemode
}

// ScoreData decodes Data as a score.updated / score.band_changed payload.
func (e *WebhookEvent) ScoreData() (*ScoreEventData, error) {
	if e.Type != WebhookScoreUpdated && e.Type != WebhookScoreBandChanged {
		return nil, fmt.Errorf("credda: event %q is not a score event", e.Type)
	}
	var d ScoreEventData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return nil, fmt.Errorf("credda: decoding score event data: %w", err)
	}
	return &d, nil
}

// DisputeData decodes Data as a dispute.resolved payload.
func (e *WebhookEvent) DisputeData() (*DisputeResolvedData, error) {
	if e.Type != WebhookDisputeResolved {
		return nil, fmt.Errorf("credda: event %q is not a dispute event", e.Type)
	}
	var d DisputeResolvedData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		return nil, fmt.Errorf("credda: decoding dispute event data: %w", err)
	}
	return &d, nil
}

// normalizeTimestamp mirrors the TypeScript SDK, which does `Number(header)`
// and then interpolates the NUMBER back into the signed message. So a header
// of "0001700000000" signs as "1700000000".
func normalizeTimestamp(raw string) (string, float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", 0, ErrInvalidTimestamp
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return strconv.FormatInt(i, 10), float64(i), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", 0, ErrInvalidTimestamp
	}
	return strconv.FormatFloat(f, 'f', -1, 64), f, nil
}

// VerifyWebhookSignature verifies a delivery's signature and, unless disabled,
// its timestamp freshness. It returns nil when the delivery is authentic, or
// one of the Err* sentinels (usable with errors.Is) otherwise.
func VerifyWebhookSignature(in VerifyWebhookInput) error {
	if in.SignatureHeader == "" {
		return ErrMissingSignature
	}
	if in.TimestampHeader == "" {
		return ErrMissingTimestamp
	}

	tsStr, tsSecs, err := normalizeTimestamp(in.TimestampHeader)
	if err != nil {
		return err
	}

	tolerance := DefaultWebhookTolerance
	if in.Tolerance != nil {
		tolerance = *in.Tolerance
	}
	if tolerance > 0 {
		now := in.Now
		if now.IsZero() {
			now = time.Now()
		}
		drift := math.Abs(float64(now.Unix()) - tsSecs)
		if drift > tolerance.Seconds() {
			return ErrTimestampSkew
		}
	}

	mac := hmac.New(sha256.New, []byte(in.Secret))
	mac.Write([]byte(tsStr + "." + in.RawBody))
	expected := []byte(hex.EncodeToString(mac.Sum(nil)))

	provided := []byte(strings.ToLower(strings.TrimSpace(strings.TrimPrefix(in.SignatureHeader, "sha256="))))
	if !hmac.Equal(expected, provided) {
		return ErrSignatureMismatch
	}
	return nil
}

// ConstructWebhookEvent verifies a delivery and returns the parsed event — the
// Stripe-style ergonomic path. It errors if verification fails or the body
// isn't the expected shape.
func ConstructWebhookEvent(in VerifyWebhookInput) (*WebhookEvent, error) {
	if err := VerifyWebhookSignature(in); err != nil {
		return nil, fmt.Errorf("credda: webhook verification failed: %w", err)
	}

	var event WebhookEvent
	if err := json.Unmarshal([]byte(in.RawBody), &event); err != nil {
		return nil, ErrInvalidBody
	}
	if event.ID == "" || event.Type == "" || event.CreatedAt == "" {
		return nil, ErrMissingFields
	}
	return &event, nil
}

// SignWebhookPayload produces the header value Credda would send for a given
// body and timestamp. Exported for tests and local fixtures — you never need
// it to receive webhooks.
func SignWebhookPayload(secret, rawBody string, timestamp int64) (signature, timestampHeader string) {
	ts := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + rawBody))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil)), ts
}
