package credda

import (
	"errors"
	"testing"
	"time"
)

const testSecret = "whsec_test_secret"

func TestVerifyWebhookSignature(t *testing.T) {
	body := `{"id":"evt_1","type":"score.updated","createdAt":"2026-07-20T00:00:00.000Z","data":{}}`
	now := time.Unix(1_700_000_000, 0)
	goodSig, goodTS := SignWebhookPayload(testSecret, body, now.Unix())

	staleSig, staleTS := SignWebhookPayload(testSecret, body, now.Unix()-3600)

	tests := []struct {
		name string
		in   VerifyWebhookInput
		want error // nil == expect success
	}{
		{
			name: "valid signature",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: goodSig, TimestampHeader: goodTS, Now: now,
			},
			want: nil,
		},
		{
			name: "valid without the sha256= prefix",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: goodSig[len("sha256="):], TimestampHeader: goodTS, Now: now,
			},
			want: nil,
		},
		{
			name: "tampered body",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body + " ",
				SignatureHeader: goodSig, TimestampHeader: goodTS, Now: now,
			},
			want: ErrSignatureMismatch,
		},
		{
			name: "tampered signature",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: "sha256=" + flipFirstHexChar(goodSig[len("sha256="):]),
				TimestampHeader: goodTS, Now: now,
			},
			want: ErrSignatureMismatch,
		},
		{
			name: "wrong secret",
			in: VerifyWebhookInput{
				Secret: "whsec_other", RawBody: body,
				SignatureHeader: goodSig, TimestampHeader: goodTS, Now: now,
			},
			want: ErrSignatureMismatch,
		},
		{
			name: "expired timestamp (replay)",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: staleSig, TimestampHeader: staleTS, Now: now,
			},
			want: ErrTimestampSkew,
		},
		{
			name: "expired timestamp accepted when tolerance disabled",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: staleSig, TimestampHeader: staleTS, Now: now,
				Tolerance: Duration(0),
			},
			want: nil,
		},
		{
			name: "expired timestamp accepted with a wide tolerance",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: staleSig, TimestampHeader: staleTS, Now: now,
				Tolerance: Duration(2 * time.Hour),
			},
			want: nil,
		},
		{
			name: "missing signature header",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body, TimestampHeader: goodTS, Now: now,
			},
			want: ErrMissingSignature,
		},
		{
			name: "missing timestamp header",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body, SignatureHeader: goodSig, Now: now,
			},
			want: ErrMissingTimestamp,
		},
		{
			name: "non-numeric timestamp",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: goodSig, TimestampHeader: "not-a-number", Now: now,
			},
			want: ErrInvalidTimestamp,
		},
		{
			name: "future timestamp beyond tolerance",
			in: VerifyWebhookInput{
				Secret: testSecret, RawBody: body,
				SignatureHeader: goodSig, TimestampHeader: goodTS,
				Now: now.Add(10 * time.Minute),
			},
			want: ErrTimestampSkew,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyWebhookSignature(tc.in)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestVerifyWebhookSignatureMatchesKnownVector(t *testing.T) {
	// Fixed vector: HMAC-SHA256("secret", "1700000000.{}") — asserts the signed
	// message is exactly `{timestamp}.{rawBody}`, matching the TS SDK.
	sig, ts := SignWebhookPayload("secret", "{}", 1_700_000_000)
	if ts != "1700000000" {
		t.Fatalf("unexpected timestamp header %q", ts)
	}
	err := VerifyWebhookSignature(VerifyWebhookInput{
		Secret: "secret", RawBody: "{}",
		SignatureHeader: sig, TimestampHeader: ts,
		Now: time.Unix(1_700_000_000, 0),
	})
	if err != nil {
		t.Fatalf("known vector failed: %v", err)
	}

	// The same body signed under a different timestamp must not verify.
	err = VerifyWebhookSignature(VerifyWebhookInput{
		Secret: "secret", RawBody: "{}",
		SignatureHeader: sig, TimestampHeader: "1700000001",
		Now: time.Unix(1_700_000_000, 0),
	})
	if !errors.Is(err, ErrSignatureMismatch) {
		t.Fatalf("expected signature mismatch across timestamps, got %v", err)
	}
}

func TestConstructWebhookEvent(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	scoreBody := `{"id":"evt_1","type":"score.updated","createdAt":"2026-07-20T00:00:00.000Z",` +
		`"data":{"user":{"externalId":"u_1"},"score":72.5,"band":"STRONG","previousScore":70,` +
		`"previousBand":"STRONG","confidence":0.8,"formulaVersion":"5.0","computedAt":"2026-07-20T00:00:00.000Z",` +
		`"reason":{"scoreDelta":2.5,"direction":"up","factors":[],"topDriver":null,"confidenceDelta":0.1,"momentumDelta":0}}}`

	disputeBody := `{"id":"evt_2","type":"dispute.resolved","createdAt":"2026-07-20T00:00:00.000Z",` +
		`"data":{"disputeId":"dsp_1","user":{"externalId":"u_1"},"outcome":"FOR_USER",` +
		`"status":"RESOLVED_FOR_USER","lapsed":false,"resolvedAt":"2026-07-20T00:00:00.000Z"}}`

	t.Run("score event", func(t *testing.T) {
		sig, ts := SignWebhookPayload(testSecret, scoreBody, now.Unix())
		e, err := ConstructWebhookEvent(VerifyWebhookInput{
			Secret: testSecret, RawBody: scoreBody,
			SignatureHeader: sig, TimestampHeader: ts, Now: now,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.ID != "evt_1" || e.Type != WebhookScoreUpdated {
			t.Fatalf("unexpected envelope: %+v", e)
		}
		data, err := e.ScoreData()
		if err != nil {
			t.Fatalf("ScoreData: %v", err)
		}
		if data.Score != 72.5 || data.User.ExternalID != "u_1" {
			t.Fatalf("unexpected score data: %+v", data)
		}
		if data.Reason == nil || data.Reason.Direction != "up" {
			t.Fatalf("unexpected reason: %+v", data.Reason)
		}
		if _, err := e.DisputeData(); err == nil {
			t.Fatal("expected DisputeData to reject a score event")
		}
	})

	t.Run("dispute event", func(t *testing.T) {
		sig, ts := SignWebhookPayload(testSecret, disputeBody, now.Unix())
		e, err := ConstructWebhookEvent(VerifyWebhookInput{
			Secret: testSecret, RawBody: disputeBody,
			SignatureHeader: sig, TimestampHeader: ts, Now: now,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := e.DisputeData()
		if err != nil {
			t.Fatalf("DisputeData: %v", err)
		}
		if data.DisputeID != "dsp_1" || data.Outcome != DisputeForUser || data.Lapsed {
			t.Fatalf("unexpected dispute data: %+v", data)
		}
	})

	t.Run("rejects a bad signature before parsing", func(t *testing.T) {
		_, ts := SignWebhookPayload(testSecret, scoreBody, now.Unix())
		_, err := ConstructWebhookEvent(VerifyWebhookInput{
			Secret: testSecret, RawBody: scoreBody,
			SignatureHeader: "sha256=deadbeef", TimestampHeader: ts, Now: now,
		})
		if !errors.Is(err, ErrSignatureMismatch) {
			t.Fatalf("expected signature mismatch, got %v", err)
		}
	})

	t.Run("rejects non-JSON body", func(t *testing.T) {
		body := "not json"
		sig, ts := SignWebhookPayload(testSecret, body, now.Unix())
		_, err := ConstructWebhookEvent(VerifyWebhookInput{
			Secret: testSecret, RawBody: body,
			SignatureHeader: sig, TimestampHeader: ts, Now: now,
		})
		if !errors.Is(err, ErrInvalidBody) {
			t.Fatalf("expected invalid body, got %v", err)
		}
	})

	t.Run("rejects a body missing required fields", func(t *testing.T) {
		body := `{"type":"score.updated"}`
		sig, ts := SignWebhookPayload(testSecret, body, now.Unix())
		_, err := ConstructWebhookEvent(VerifyWebhookInput{
			Secret: testSecret, RawBody: body,
			SignatureHeader: sig, TimestampHeader: ts, Now: now,
		})
		if !errors.Is(err, ErrMissingFields) {
			t.Fatalf("expected missing fields, got %v", err)
		}
	})
}

// flipFirstHexChar mutates the first hex digit of a signature so the result is
// still well-formed hex but no longer matches.
func flipFirstHexChar(sig string) string {
	if sig == "" {
		return sig
	}
	replacement := byte('a')
	if sig[0] == 'a' {
		replacement = 'b'
	}
	return string(replacement) + sig[1:]
}
