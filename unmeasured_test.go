package credda

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// UNMEASURED IS NOT ZERO: the Go half of the rule.
//
// encoding/json decodes a JSON null into a non-pointer float64 as a NO-OP: the
// field keeps its zero value and no error is returned. A component the API never
// measured would land as Score 0.0, and in a product whose bands run down to At
// Risk a 0.0 is the worst possible record rather than an unknown one.
//
// Every nullable score, band and rate is therefore a POINTER, so the absence
// survives decoding as nil and the compiler forces a reader to say what nil
// means. The DISCRIMINATORS (Available, InsufficientData, DataState) travel
// beside the value and say WHY it was not measured. These tests pin, in the same
// three shapes every language binding pins:
//   1. available:false + a null score preserves BOTH facts after decoding;
//   2. a MEASURED zero stays distinguishable from an unmeasured one, and stays
//      displayable, because a real 0 is real bad news and must not be hidden;
//   3. a payload from an OLDER API that omits the field decodes safely.

func serveJSON(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithAPIKey("crd_live_test"), WithBaseURL(srv.URL))
}

func TestScoreComponentsKeepsAvailableAndNullScoreAsTwoFacts(t *testing.T) {
	c := serveJSON(t, `{
		"userId": "never_scored",
		"available": true,
		"finalScore": 20,
		"scoreBand": "Unproven",
		"components": [
			{"key":"reliability","label":"Reliability","score":null,"weight":0.4,"available":false,"description":"Not measured."}
		],
		"dataSufficiency": {
			"insufficientData": true,
			"state": "no_recorded_outcomes",
			"recordedOutcomes": 0,
			"verifiedOutcomes": 0,
			"note": "No outcomes recorded yet."
		}
	}`)

	out, err := c.GetScoreComponents(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetScoreComponents: %v", err)
	}
	if len(out.Components) != 1 {
		t.Fatalf("want 1 component, got %d", len(out.Components))
	}
	comp := out.Components[0]

	// The whole point: the null survives as nil, so the absence is not
	// impersonated by a 0.0 that a reader would render as the worst band.
	if comp.Score != nil {
		t.Fatalf("a JSON null must decode as nil (absent); got %v", *comp.Score)
	}
	if comp.Available {
		t.Fatal("available:false must survive decoding; without it 0.0 reads as a measured zero")
	}
	if out.DataSufficiency == nil {
		t.Fatal("dataSufficiency must decode; it is what a consumer renders instead of a rate")
	}
	if !out.DataSufficiency.InsufficientData {
		t.Fatal("insufficientData must be true for a record with no outcomes")
	}
	if out.DataSufficiency.State != DataStateNoRecordedOutcomes {
		t.Fatalf("state = %q, want %q", out.DataSufficiency.State, DataStateNoRecordedOutcomes)
	}
}

func TestScoreComponentsMeasuredZeroStaysDistinctFromUnmeasured(t *testing.T) {
	c := serveJSON(t, `{
		"userId": "measured_badly",
		"available": true,
		"finalScore": 20,
		"scoreBand": "At Risk",
		"components": [
			{"key":"reliability","label":"Reliability","score":0,"weight":0.4,"available":true,"description":"Completed none of them."},
			{"key":"timeliness","label":"Timeliness","score":null,"weight":0.35,"available":false,"description":"Not measured."}
		],
		"dataSufficiency": {"insufficientData": false, "state": "ok", "recordedOutcomes": 12, "verifiedOutcomes": 12, "note": ""}
	}`)

	out, err := c.GetScoreComponents(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetScoreComponents: %v", err)
	}
	measured, unmeasured := out.Components[0], out.Components[1]

	// The two are now distinguishable on the VALUE itself, not only on the flag:
	// a genuine 0 over a real denominator is real bad news that must still show,
	// and an unmeasured component is nil.
	if measured.Score == nil {
		t.Fatal("a MEASURED zero must decode as a non-nil pointer to 0, not as absent")
	}
	if *measured.Score != 0 {
		t.Fatalf("measured score = %v, want 0", *measured.Score)
	}
	if unmeasured.Score != nil {
		t.Fatalf("an unmeasured component must decode as nil; got %v", *unmeasured.Score)
	}
	if !measured.Available {
		t.Fatal("a MEASURED zero must report available:true and remain displayable")
	}
	if unmeasured.Available {
		t.Fatal("an unmeasured component must report available:false")
	}
	if out.DataSufficiency == nil || out.DataSufficiency.InsufficientData {
		t.Fatal("a record with 12 outcomes is measured; insufficientData must be false")
	}
}

func TestScoreComponentsDecodesAgainstAnOlderAPI(t *testing.T) {
	// A consumer pinned to an API that predates the fields: no error, no panic,
	// and the absent flag is Go's zero value. Read it as "unknown", never as a
	// positive assertion that the component is unavailable.
	c := serveJSON(t, `{
		"userId": "old_api",
		"available": true,
		"components": [{"key":"reliability","label":"Reliability","score":78,"weight":0.4,"description":"Good."}]
	}`)

	out, err := c.GetScoreComponents(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("an older payload must decode without error: %v", err)
	}
	if out.DataSufficiency != nil {
		t.Fatal("an absent dataSufficiency must stay nil, not become a zero-valued struct")
	}
	if out.Components[0].Score == nil || *out.Components[0].Score != 78 {
		t.Fatalf("real score lost: %v", out.Components[0].Score)
	}
}

func TestScoreExplainCarriesDataSufficiencyAndInformationalReasonCode(t *testing.T) {
	c := serveJSON(t, `{
		"summary": "No outcomes recorded yet.",
		"factors": [
			{"key":"completionRate","name":"Completion Rate","value":null,"weight":0.37,"weightPercent":"37%","contribution":null,"available":false,"description":"Not measured."}
		],
		"dataSufficiency": {"insufficientData": true, "state": "no_recorded_outcomes", "recordedOutcomes": 0, "verifiedOutcomes": 0, "note": "n/a"},
		"reasonCodes": {
			"formulaVersion": "5.3",
			"reasonCodesVersion": "1.2",
			"finalScore": null,
			"method": "importance-weighted",
			"keyFactorLimit": 4,
			"adverseActionReasons": [],
			"supportingFactors": [],
			"informationalFactors": [
				{"code":"NO_RECORDED_OUTCOMES","factor":"data","direction":"informational","title":"No recorded outcomes","description":"","contribution":0,"rank":1,"evidence":{}}
			],
			"insufficientData": true,
			"dataState": "no_recorded_outcomes",
			"disclosures": [],
			"advisory": ""
		},
		"confidence": {"eventsRecorded": 0, "eventsNeededForFull": 6, "level": "None"}
	}`)

	out, err := c.GetScoreExplain(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetScoreExplain: %v", err)
	}
	if out.DataSufficiency == nil || !out.DataSufficiency.InsufficientData {
		t.Fatal("dataSufficiency must decode and report insufficientData")
	}
	if out.Factors[0].Available {
		t.Fatal("a factor with a null value must report available:false")
	}
	if out.Factors[0].Value != 0 {
		t.Fatalf("null value decodes to 0.0 in Go; got %v", out.Factors[0].Value)
	}
	if out.Factors[0].Key != "completionRate" {
		t.Fatalf("stable key lost: %q", out.Factors[0].Key)
	}

	rc := out.ReasonCodes
	if rc == nil {
		t.Fatal("reasonCodes must decode: it is where insufficientData/dataState live")
	}
	// The load-bearing invariant: an absent measurement yields NO adverse reason.
	if !rc.InsufficientData {
		t.Fatal("insufficientData must be true")
	}
	if rc.DataState != DataStateNoRecordedOutcomes {
		t.Fatalf("dataState = %q", rc.DataState)
	}
	if len(rc.AdverseActionReasons) != 0 || len(rc.SupportingFactors) != 0 {
		t.Fatal("both ranked lists must be empty when nothing is attributable")
	}
	if len(rc.InformationalFactors) != 1 || rc.InformationalFactors[0].Direction != ReasonDirectionInformational {
		t.Fatal("the informational code must survive decoding with its own direction")
	}
	if rc.FinalScore != nil {
		t.Fatalf("finalScore must stay nil for an unscored subject; got %v", *rc.FinalScore)
	}
}

func TestScoreExplainDistinguishesPendingComputationFromEmptyRecord(t *testing.T) {
	c := serveJSON(t, `{
		"summary": "",
		"factors": [],
		"reasonCodes": {
			"formulaVersion":"5.3","reasonCodesVersion":"1.2","finalScore":null,"method":"","keyFactorLimit":4,
			"adverseActionReasons":[],"supportingFactors":[],
			"informationalFactors":[{"code":"SCORE_NOT_YET_COMPUTED","factor":"data","direction":"informational","title":"","description":"","contribution":0,"rank":1,"evidence":{}}],
			"insufficientData": true, "dataState": "score_not_yet_computed", "disclosures": [], "advisory": ""
		},
		"confidence": {"eventsRecorded": 3, "eventsNeededForFull": 6, "level": "None"}
	}`)

	out, err := c.GetScoreExplain(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetScoreExplain: %v", err)
	}
	if out.ReasonCodes.DataState != DataStateScoreNotYetComputed {
		t.Fatalf("dataState = %q, want %q", out.ReasonCodes.DataState, DataStateScoreNotYetComputed)
	}
	if out.ReasonCodes.DataState == DataStateNoRecordedOutcomes {
		t.Fatal("a pending computation is not an empty record")
	}
}

func TestTrustSummaryEvidenceCarriesInsufficientData(t *testing.T) {
	c := serveJSON(t, `{
		"userId":"u_1","available":true,"summary":"",
		"evidence":{"finalScore":20,"scoreBand":"Unproven","confidenceLevel":"none",
			"completionRate":null,"onTimeRate":null,"verifiedEvents":0,"totalEvents":0,
			"distinctPlatforms":0,"insufficientData":true}
	}`)
	out, err := c.GetTrustSummary(context.Background(), "u_1", false)
	if err != nil {
		t.Fatalf("GetTrustSummary: %v", err)
	}
	if out.Evidence == nil || !out.Evidence.InsufficientData {
		t.Fatal("insufficientData must decode on the evidence block")
	}
	if out.Evidence.CompletionRate != nil {
		t.Fatalf("a null completionRate must decode as nil; got %v", *out.Evidence.CompletionRate)
	}
	if out.Evidence.OnTimeRate != nil {
		t.Fatalf("a null onTimeRate must decode as nil; got %v", *out.Evidence.OnTimeRate)
	}

	// A record that genuinely completed nothing: same 0.0, opposite meaning.
	c2 := serveJSON(t, `{
		"userId":"u_2","available":true,"summary":"",
		"evidence":{"finalScore":20,"scoreBand":"At Risk","confidenceLevel":"high",
			"completionRate":0,"onTimeRate":0,"verifiedEvents":8,"totalEvents":8,
			"distinctPlatforms":1,"insufficientData":false}
	}`)
	measured, err := c2.GetTrustSummary(context.Background(), "u_2", false)
	if err != nil {
		t.Fatalf("GetTrustSummary: %v", err)
	}
	if measured.Evidence.InsufficientData {
		t.Fatal("a measured zero must report insufficientData:false and stay displayable")
	}
	// A measured zero is present-and-zero; the unmeasured one is absent. The two
	// are distinguishable from the value alone.
	if measured.Evidence.CompletionRate == nil {
		t.Fatal("a measured 0% completion must stay a non-nil pointer to 0 and stay displayable")
	}
	if *measured.Evidence.CompletionRate != 0 {
		t.Fatalf("measured completionRate = %v, want 0", *measured.Evidence.CompletionRate)
	}
	if out.Evidence.CompletionRate != nil {
		t.Fatal("the unmeasured record must stay nil, so it cannot be confused with the measured zero")
	}
}

func TestReliabilityReportMetricsCarryInsufficientDataAndDataState(t *testing.T) {
	c := serveJSON(t, `{
		"userId":"u_1","reliabilityReportVersion":"1.0","note":"",
		"reliability":{"score":null,"band":null,"confidence":0,"formulaVersion":"5.3","reasonCodesVersion":"1.2"},
		"metrics":{"completionRate":null,"onTimeRate":null,"consistency":null,"recency":null,"disputeRate":null,
			"insufficientData":true,"dataState":"score_not_yet_computed"},
		"verifiedExperience":{},"topFactors":[],"recentOutcomes":[],
		"benchmark":null,"status":{"scoreFrozen":false},
		"provenance":{"formulaVersion":"5.3","computedAt":null},
		"disclosures":[],"advisory":""
	}`)

	out, err := c.GetReliabilityReport(context.Background(), "u_1", nil, false)
	if err != nil {
		t.Fatalf("GetReliabilityReport: %v", err)
	}
	if !out.Metrics.InsufficientData {
		t.Fatal("insufficientData must decode on the metrics block")
	}
	if out.Metrics.DataState != DataStateScoreNotYetComputed {
		t.Fatalf("dataState = %q", out.Metrics.DataState)
	}
	// Every rate on the block is null on the wire, so every one of them must be
	// nil here. None may impersonate a measurement.
	if out.Metrics.CompletionRate != nil || out.Metrics.OnTimeRate != nil ||
		out.Metrics.Consistency != nil || out.Metrics.DisputeRate != nil ||
		out.Metrics.Recency != nil {
		t.Fatalf("every null rate must decode as nil; got %+v", out.Metrics)
	}
	// The reliability block is null too: no score, and no band to render.
	if out.Reliability.Score != nil {
		t.Fatalf("a null score must decode as nil; got %v", *out.Reliability.Score)
	}
	if out.Reliability.Band != nil {
		t.Fatalf("a null band must decode as nil, NOT as \"\"; got %q", *out.Reliability.Band)
	}
	// topFactors is empty because nothing is attributable, NOT because the record
	// is clean. Without the flag those two are indistinguishable.
	if len(out.TopFactors) != 0 {
		t.Fatal("no factors are attributable for an unmeasured record")
	}
}

func TestProjectionTimelinessReportsAnUpperBound(t *testing.T) {
	c := serveJSON(t, `{
		"userId":"u_1","delta":4,
		"current":{"finalScore":20,"scoreBand":"Unproven"},
		"projected":{"finalScore":24,"scoreBand":"Provisional"},
		"bandChanged":true,"formulaVersion":"5.3",
		"timeliness":{"statedEvents":0,"unstatedEvents":2,"basis":"unstated",
			"projectionIsUpperBound":true,"note":"No lateness was stated and none was assumed.",
			"projectedIfUnstatedWereLate":{"finalScore":21,"scoreBand":"Unproven","delta":1}}
	}`)

	out, err := c.ProjectScore(context.Background(), "u_1", []ProjectionEventInput{{EventType: "CONTRACT_FULFILLED"}})
	if err != nil {
		t.Fatalf("ProjectScore: %v", err)
	}
	if out.Timeliness == nil {
		t.Fatal("timeliness must decode")
	}
	if !out.Timeliness.ProjectionIsUpperBound {
		t.Fatal("an unstated lateness makes the projection a best case, not a point estimate")
	}
	if out.Timeliness.ProjectedIfUnstatedWereLate == nil ||
		out.Timeliness.ProjectedIfUnstatedWereLate.FinalScore != 21 {
		t.Fatal("the other end of the bound must decode")
	}
}

// A round trip through the SDK's own structs must not turn a null into a zero
// on the way back out either: Available/InsufficientData re-encode faithfully.
func TestDiscriminatorsRoundTripThroughEncoding(t *testing.T) {
	in := ScoreComponent{Key: "reliability", Label: "Reliability", Available: false}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ScoreComponent
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Available {
		t.Fatal("available must survive a round trip; omitempty would silently drop it")
	}
	if got := string(raw); !contains(got, `"available":false`) {
		t.Fatalf("available must serialise explicitly, got %s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// ─── The nullability change: absent, present-and-zero, and telling them apart ──
//
// The three tests below are the proof obligation for making the eleven nullable
// scores, bands and rates POINTERS. Each one runs the identical payload twice,
// once with every value null and once with every value a genuine 0, and asserts
// that a consumer can tell the two apart from the decoded value alone. That is
// the property that was impossible before: both shapes used to decode to 0.0.

func TestProfessionalRecordNullScoreIsAbsentNotZero(t *testing.T) {
	unscored := serveJSON(t, `{
		"userId":"never_scored","professionalRecordVersion":"1.0",
		"reliability":{"score":null,"band":null,"confidence":0},
		"verifiedExperience":{"verifiedOutcomes":0,"totalOutcomes":0,"verificationDepth":null,"verifiedPlatforms":0},
		"tenure":{"firstRecordedAt":null,"trackRecordDays":null,"trackRecordMonths":null},
		"status":{"scoreFrozen":false},
		"provenance":{"formulaVersion":"5.3","computedAt":null},
		"disclosures":[]
	}`)
	out, err := unscored.GetProfessionalRecord(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("GetProfessionalRecord: %v", err)
	}
	// ABSENT. Not 0, and crucially not "" for the band: an empty band would be
	// rendered by a switch as an unknown label, or worse, defaulted to the floor.
	if out.Reliability.Score != nil {
		t.Fatalf("a null score must decode as nil; got %v", *out.Reliability.Score)
	}
	if out.Reliability.Band != nil {
		t.Fatalf("a null band must decode as nil, NOT as \"\"; got %q", *out.Reliability.Band)
	}

	// PRESENT AND ZERO: a subject scored at the very bottom of the scale. This is
	// a real measurement and must survive as one.
	scoredAtZero := serveJSON(t, `{
		"userId":"rock_bottom","professionalRecordVersion":"1.0",
		"reliability":{"score":0,"band":"At Risk","confidence":0.9},
		"verifiedExperience":{"verifiedOutcomes":11,"totalOutcomes":11,"verificationDepth":1,"verifiedPlatforms":2},
		"tenure":{"firstRecordedAt":"2025-01-01T00:00:00Z","trackRecordDays":400,"trackRecordMonths":13.1},
		"status":{"scoreFrozen":false},
		"provenance":{"formulaVersion":"5.3","computedAt":"2026-08-01T00:00:00Z"},
		"disclosures":[]
	}`)
	worst, err := scoredAtZero.GetProfessionalRecord(context.Background(), "u_2")
	if err != nil {
		t.Fatalf("GetProfessionalRecord: %v", err)
	}
	if worst.Reliability.Score == nil {
		t.Fatal("a measured 0 must decode as a non-nil pointer to 0, never as absent")
	}
	if *worst.Reliability.Score != 0 {
		t.Fatalf("measured score = %v, want 0", *worst.Reliability.Score)
	}
	if worst.Reliability.Band == nil || *worst.Reliability.Band != "At Risk" {
		t.Fatal("a measured worst-case band must remain displayable")
	}

	// DISTINGUISHABLE: the exact discrimination a caller has to make before it
	// decides whether to show a number or to show "no record".
	if (out.Reliability.Score == nil) == (worst.Reliability.Score == nil) {
		t.Fatal("no record and a measured 0 must not decode to the same thing")
	}
}

func TestReliabilityReportNullRatesAreAbsentNotZero(t *testing.T) {
	body := func(rates string) string {
		return `{
			"userId":"u_1","reliabilityReportVersion":"1.0","note":"",
			"reliability":` + rates + `,
			"metrics":` + rates + `,
			"verifiedExperience":{},"topFactors":[],"recentOutcomes":[],
			"benchmark":null,"status":{"scoreFrozen":false},
			"provenance":{"formulaVersion":"5.3","computedAt":null},
			"disclosures":[],"advisory":""
		}`
	}
	nulls := serveJSON(t, body(`{"score":null,"band":null,"confidence":0,
		"completionRate":null,"onTimeRate":null,"consistency":null,"recency":null,"disputeRate":null,
		"insufficientData":true,"dataState":"no_recorded_outcomes"}`))
	absent, err := nulls.GetReliabilityReport(context.Background(), "u_1", nil, false)
	if err != nil {
		t.Fatalf("GetReliabilityReport: %v", err)
	}
	for name, got := range map[string]*float64{
		"completionRate": absent.Metrics.CompletionRate,
		"onTimeRate":     absent.Metrics.OnTimeRate,
		"consistency":    absent.Metrics.Consistency,
		"disputeRate":    absent.Metrics.DisputeRate,
		"score":          absent.Reliability.Score,
	} {
		if got != nil {
			t.Fatalf("null %s must decode as nil; got %v", name, *got)
		}
	}
	if absent.Reliability.Band != nil {
		t.Fatalf("null band must decode as nil; got %q", *absent.Reliability.Band)
	}

	zeros := serveJSON(t, body(`{"score":0,"band":"At Risk","confidence":0.9,
		"completionRate":0,"onTimeRate":0,"consistency":0,"recency":0,"disputeRate":0,
		"insufficientData":false,"dataState":"ok"}`))
	measured, err := zeros.GetReliabilityReport(context.Background(), "u_2", nil, false)
	if err != nil {
		t.Fatalf("GetReliabilityReport: %v", err)
	}
	for name, got := range map[string]*float64{
		"completionRate": measured.Metrics.CompletionRate,
		"onTimeRate":     measured.Metrics.OnTimeRate,
		"consistency":    measured.Metrics.Consistency,
		"disputeRate":    measured.Metrics.DisputeRate,
		"score":          measured.Reliability.Score,
	} {
		if got == nil {
			t.Fatalf("a measured 0 %s must stay present and displayable, not become absent", name)
		}
		if *got != 0 {
			t.Fatalf("measured %s = %v, want 0", name, *got)
		}
	}

	// A caller rendering "0% completion" must reach that branch ONLY for the
	// second payload. This is the whole defect, stated as an assertion.
	render := func(m *ReliabilityReportMetrics) string {
		if m.CompletionRate == nil {
			return "not measured"
		}
		return "0% completion"
	}
	if render(&absent.Metrics) != "not measured" {
		t.Fatal("an unmeasured record must never render as a rate")
	}
	if render(&measured.Metrics) != "0% completion" {
		t.Fatal("a genuinely measured 0 must still render as a rate")
	}
}

// A null must also survive the trip BACK OUT as an explicit null. If any of
// these fields ever gained omitempty, a re-serialised payload would drop the key
// entirely and a downstream reader would see "absent field" rather than "the API
// said null". Go would decode that back to the same nil, but a non-Go consumer
// of the re-encoded JSON would lose the distinction.
func TestNullRatesReSerialiseAsExplicitNull(t *testing.T) {
	raw, err := json.Marshal(ReliabilityReportMetrics{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{
		`"completionRate":null`, `"onTimeRate":null`,
		`"consistency":null`, `"disputeRate":null`,
	} {
		if !contains(string(raw), key) {
			t.Fatalf("want %s in %s", key, raw)
		}
	}

	rawEvidence, err := json.Marshal(TrustSummaryEvidence{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"completionRate":null`, `"onTimeRate":null`} {
		if !contains(string(rawEvidence), key) {
			t.Fatalf("want %s in %s", key, rawEvidence)
		}
	}

	rawRecord, err := json.Marshal(ProfessionalRecordReliability{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"score":null`, `"band":null`} {
		if !contains(string(rawRecord), key) {
			t.Fatalf("want %s in %s", key, rawRecord)
		}
	}

	rawComponent, err := json.Marshal(ScoreComponent{Key: "reliability"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !contains(string(rawComponent), `"score":null`) {
		t.Fatalf("want \"score\":null in %s", rawComponent)
	}
}

// A NUMERIC weight must DECODE, because a string declaration did not fail
// softly here: it failed the whole call.
//
// The API split this field on 2026-08-09 (#462) into a numeric `weight` and a
// separate `weightPercent` label. ScoreExplainFactor.Weight stayed `string`, and
// encoding/json does not tolerate that the way it tolerates a null into a
// float64: a number into a string is an UnmarshalTypeError, `do` wraps it, and
// GetScoreExplain returned (nil, error) for EVERY caller against the live API.
// The whole adverse-action surface was unreachable from Go.
//
// The suite did not catch it because its own fixture still sent the pre-split
// shape, "weight":"40%". A test that pins the old wire format passes forever
// while production is broken, so this one is written against the shape
// services/scoreExplain.ts actually emits.
func TestScoreExplainDecodesTheNumericWeightTheAPIActuallySends(t *testing.T) {
	c := serveJSON(t, `{
		"summary": "Strong record.",
		"factors": [
			{"key":"completionRate","name":"Completion Rate","value":0.9,"weight":0.37,"weightPercent":"37%","contribution":0.33,"available":true,"description":"Strong."},
			{"key":"onTimeRate","name":"On-time Rate","value":0.8,"weight":0.32,"weightPercent":"32%","contribution":0.26,"available":true,"description":"Good."}
		]
	}`)

	out, err := c.GetScoreExplain(context.Background(), "u_1")
	if err != nil {
		t.Fatalf("the live wire shape must decode, got: %v", err)
	}
	if len(out.Factors) != 2 {
		t.Fatalf("factors = %d, want 2", len(out.Factors))
	}
	if out.Factors[0].Weight != 0.37 {
		t.Errorf("Weight = %v, want 0.37", out.Factors[0].Weight)
	}
	if out.Factors[0].WeightPercent != "37%" {
		t.Errorf("WeightPercent = %q, want \"37%%\"", out.Factors[0].WeightPercent)
	}
	// The weights must still be usable as the arithmetic they are: this is the
	// thing a string could never do without a parse the caller had to write.
	total := 0.0
	for _, f := range out.Factors {
		total += f.Weight
	}
	if total < 0.68 || total > 0.70 {
		t.Errorf("summed weights = %v, want ~0.69", total)
	}
}

// Non-vacuity: the OLD fixture shape is what let this hide. If the wire ever
// goes back to a string weight this fails, which is the correct alarm either
// way, because the field would have changed type again.
func TestScoreExplainRejectsTheRetiredStringWeightShape(t *testing.T) {
	c := serveJSON(t, `{"summary":"s","factors":[{"key":"completionRate","name":"Completion Rate","value":0.9,"weight":"37%","contribution":0.33,"available":true,"description":"d"}]}`)

	if _, err := c.GetScoreExplain(context.Background(), "u_1"); err == nil {
		t.Fatal("a string weight must not decode silently; the field is numeric now")
	}
}
