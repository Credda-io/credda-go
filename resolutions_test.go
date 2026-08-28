package credda

import (
	"context"
	"testing"
)

// The resolution record, decoded. This file replaces earnings_test.go, which
// covered a surface of the retired trust product.
//
// A resolution is the densest thing this API serves and the one a human
// actually reads before trusting a fix. Every test here is about a distinction
// the record draws that a careless client would erase.

// The complete record: reproduced, diagnosed, fixed, verified, with a
// regression test that fails before and passes after. This is what the product
// is for, and the client must carry all of it.
func TestGetResolutionDecodesAnEstablishedRecord(t *testing.T) {
	c := serveJSON(t, `{"resolution":{
		"id":"res_1","investigationId":"inv_1",
		"bug":{
			"reported":"Checkout returns 500 on submit",
			"reference":"https://tracker.example/ISSUE-41",
			"signalId":"sig_1",
			"affectedFiles":["src/checkout/total.ts"]
		},
		"evidence":[
			{"id":"ev_1","type":"REPRODUCTION","summary":"POST /checkout total 1500 -> 500"},
			{"id":"ev_2","type":"STACK_TRACE","summary":"AssertionError in total.ts:88"}
		],
		"reproduction":{
			"status":"REPRODUCED",
			"command":"npm test -- checkout.spec.ts",
			"signature":{
				"command":"npm test -- checkout.spec.ts","exitCode":1,
				"errorClass":"AssertionError","normalizedMessage":"expected 200, got 500",
				"originFile":"src/checkout/total.ts","originLine":88,
				"failingTestIds":["checkout > over 1000"],"hash":"h_9f"
			},
			"evidenceId":"ev_1",
			"timedOutAttempts":0
		},
		"rootCause":{
			"hypothesisId":"hyp_1",
			"description":"sumLineItems accumulates minor units in a 32-bit int",
			"supportingEvidenceIds":["ev_1","ev_2"],
			"anchoredFiles":["src/checkout/total.ts"]
		},
		"fix":{
			"patchId":"pat_1","attempt":1,
			"filesChanged":["src/checkout/total.ts"],
			"insertions":3,"deletions":1,
			"rationale":"Accumulate into a 64-bit value; the overflow was the 500."
		},
		"verification":{
			"verificationRunId":"ver_1","verdict":"VERIFIED",
			"signals":{
				"reproductionBefore":"FAIL","reproductionAfter":"PASS",
				"regressionTestBefore":"FAIL","regressionTestAfter":"PASS",
				"existingTests":{"passed":812,"failed":0,"total":812},
				"build":"PASS","typecheck":"PASS","lint":"PASS"
			},
			"evidenceIds":["ev_4","ev_5"]
		},
		"regressionProtection":{"status":"PROTECTED","before":"FAIL","after":"PASS"},
		"confidence":{"class":"ESTABLISHED","notEstablished":[]},
		"createdAt":"2026-08-27T10:05:00.000Z"
	}}`)

	r, err := c.GetResolution(context.Background(), "res_1")
	if err != nil {
		t.Fatalf("GetResolution: %v", err)
	}

	// The report's own title, quoted, never Credda's description of the defect.
	if r.Bug.Reported != "Checkout returns 500 on submit" {
		t.Errorf("Reported = %q", r.Bug.Reported)
	}
	if r.RootCause == nil || r.Fix == nil || r.Verification == nil {
		t.Fatalf("a complete record lost a section: cause=%v fix=%v verify=%v",
			r.RootCause, r.Fix, r.Verification)
	}
	if r.Fix.Insertions != 3 || r.Fix.Deletions != 1 {
		t.Errorf("fix diffstat = +%d-%d", r.Fix.Insertions, r.Fix.Deletions)
	}

	// THE CLAIM. The before/after pair is what separates a fixed failure from a
	// mutated one, and it must survive whole. A verdict without its signals is
	// a model's opinion; with them it is a mechanical consequence.
	s := r.Verification.Signals
	if s.ReproductionBefore != "FAIL" || s.ReproductionAfter != "PASS" {
		t.Errorf("reproduction before/after = %s/%s, want FAIL/PASS", s.ReproductionBefore, s.ReproductionAfter)
	}
	if s.RegressionTestBefore != "FAIL" || s.RegressionTestAfter != "PASS" {
		t.Errorf("regression before/after = %s/%s, want FAIL/PASS", s.RegressionTestBefore, s.RegressionTestAfter)
	}
	if s.ExistingTests == nil || s.ExistingTests.Failed != 0 || s.ExistingTests.Total != 812 {
		t.Errorf("existing tests = %+v", s.ExistingTests)
	}
	if r.RegressionProtection.Before != "FAIL" || r.RegressionProtection.After != "PASS" {
		t.Errorf("regressionProtection = %+v", r.RegressionProtection)
	}

	// ESTABLISHED means the gap list is empty. Both halves are one fact.
	if r.Confidence.Class != "ESTABLISHED" || len(r.Confidence.NotEstablished) != 0 {
		t.Errorf("confidence = %+v", r.Confidence)
	}
}

// The record this product exists to be honest about: the failure happened, and
// nothing else was established. Every absent section must decode to nil, and
// the gaps must arrive with the class. A client that read a missing fix as an
// empty fix would render "0 files changed" where the truth is "no fix exists".
func TestPartiallyEstablishedRecordKeepsItsHolesAsHoles(t *testing.T) {
	c := serveJSON(t, `{"resolution":{
		"id":"res_2","investigationId":"inv_2",
		"bug":{"reported":"Search returns stale results","reference":null,"signalId":null,"affectedFiles":[]},
		"evidence":[{"id":"ev_9","type":"LOG","summary":"cache key omits the locale"}],
		"reproduction":{
			"status":"REPRODUCED","command":"curl -s localhost:3000/search?q=a",
			"signature":null,"evidenceId":"ev_9","timedOutAttempts":2
		},
		"rootCause":null,
		"fix":null,
		"verification":null,
		"regressionProtection":{"status":"UNPROTECTED","before":"NOT_RUN","after":"NOT_RUN"},
		"confidence":{"class":"PARTIALLY_ESTABLISHED","notEstablished":[
			"No cause was established from the captured evidence.",
			"No change was written, so nothing was verified."
		]},
		"createdAt":"2026-08-27T11:00:00.000Z"
	}}`)

	r, err := c.GetResolution(context.Background(), "res_2")
	if err != nil {
		t.Fatalf("GetResolution: %v", err)
	}
	if r.RootCause != nil {
		t.Error("a missing root cause decoded to a non-nil value")
	}
	if r.Fix != nil {
		t.Errorf("a missing fix decoded to %+v; nil is the only honest reading", r.Fix)
	}
	if r.Verification != nil {
		t.Error("a missing verification decoded to a non-nil value")
	}
	// The gaps are the record. Losing them turns a qualified answer into a
	// bare assertion.
	if len(r.Confidence.NotEstablished) != 2 {
		t.Fatalf("notEstablished = %v, want both gaps", r.Confidence.NotEstablished)
	}
	// A reproduction that ran with no capture is a real state; the client must
	// not require a signature to decode the section.
	if r.Reproduction.Signature != nil {
		t.Error("a null signature decoded to a non-nil value")
	}
	if r.Reproduction.TimedOutAttempts == nil || *r.Reproduction.TimedOutAttempts != 2 {
		t.Errorf("TimedOutAttempts = %v, want 2", r.Reproduction.TimedOutAttempts)
	}
}

// "Nothing verified this" and "something verified this and it failed" are
// different answers. VerificationVerdict is nil for the first and never a
// stand-in.
func TestListResolutionsKeepsANullVerdictNull(t *testing.T) {
	c := serveJSON(t, `{
		"resolutions":[
			{"id":"res_2","investigationId":"inv_2","reported":"Search returns stale results",
			 "reference":null,"signalId":null,"reproductionStatus":"REPRODUCED",
			 "verificationVerdict":null,"regressionStatus":"UNPROTECTED",
			 "confidenceClass":"PARTIALLY_ESTABLISHED",
			 "notEstablished":["No change was written, so nothing was verified."],
			 "createdAt":"2026-08-27T11:00:00.000Z"},
			{"id":"res_3","investigationId":"inv_3","reported":"Login loops",
			 "reference":null,"signalId":null,"reproductionStatus":"REPRODUCED",
			 "verificationVerdict":"REJECTED","regressionStatus":"UNPROTECTED",
			 "confidenceClass":"NOT_ESTABLISHED","notEstablished":["The patch failed typecheck."],
			 "createdAt":"2026-08-27T11:30:00.000Z"}
		],
		"total":2
	}`)

	out, err := c.ListResolutions(context.Background(), &ResolutionQuery{Confidence: "NOT_ESTABLISHED"})
	if err != nil {
		t.Fatalf("ListResolutions: %v", err)
	}
	if out.Resolutions[0].VerificationVerdict != nil {
		t.Errorf("an unverified record reported the verdict %q",
			*out.Resolutions[0].VerificationVerdict)
	}
	if v := out.Resolutions[1].VerificationVerdict; v == nil || *v != "REJECTED" {
		t.Errorf("VerificationVerdict = %v, want REJECTED", v)
	}
	// Every summary row carries its gaps. A queue that shows the class alone is
	// showing exactly the bare assertion the pairing prevents.
	for _, row := range out.Resolutions {
		if len(row.NotEstablished) == 0 {
			t.Errorf("%s: class %s arrived with no gaps behind it", row.ID, row.ConfidenceClass)
		}
	}
}

// `{"resolution": null}` with a 200 means the investigation EXISTS and has
// produced nothing yet — the answer an investigation page renders. It must not
// come back as an error, and it must not come back as an empty Resolution
// either: a zero-valued record would render as a resolution with no bug, no
// evidence and no confidence class.
func TestLatestResolutionNullIsAnAnswerNotAnError(t *testing.T) {
	c := serveJSON(t, `{"resolution":null}`)
	r, err := c.LatestResolution(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("LatestResolution: %v", err)
	}
	if r != nil {
		t.Errorf("a null resolution decoded to %+v; nil is the answer", r)
	}
}

func TestLatestResolutionReturnsTheRecordWhenThereIsOne(t *testing.T) {
	c := serveJSON(t, `{"resolution":{
		"id":"res_1","investigationId":"inv_1",
		"bug":{"reported":"x","reference":null,"signalId":null,"affectedFiles":[]},
		"evidence":[],
		"reproduction":{"status":"REPRODUCED","command":null,"signature":null,"evidenceId":null,"timedOutAttempts":null},
		"rootCause":null,"fix":null,"verification":null,
		"regressionProtection":{"status":"UNPROTECTED","before":"NOT_RUN","after":"NOT_RUN"},
		"confidence":{"class":"NOT_ESTABLISHED","notEstablished":["nothing"]},
		"createdAt":"2026-08-27T11:00:00.000Z"
	}}`)

	r, err := c.LatestResolution(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("LatestResolution: %v", err)
	}
	if r == nil || r.ID != "res_1" {
		t.Fatalf("got %+v", r)
	}
}
