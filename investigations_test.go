package credda

import (
	"context"
	"encoding/json"
	"testing"
)

// The investigation surface, decoded. This file replaces trustgateway_test.go,
// which covered the retired trust product's own surface.

func TestGetInvestigationDecodesTheWholeRecord(t *testing.T) {
	c := serveJSON(t, `{
		"investigation": {
			"id":"inv_1","orgId":"org_1","repositoryId":"repo_1",
			"issueRef":"https://tracker.example/ISSUE-41",
			"issueTitle":"Checkout returns 500 on submit",
			"issueBody":"POST /checkout 500s for cart totals over 1000.",
			"state":"REPRODUCED_AND_DIAGNOSED",
			"outcome":"REPRODUCED_AND_DIAGNOSED",
			"providerId":"anthropic",
			"budget":{"maxUsd":2.5,"spentUsd":0.41},
			"startedAt":"2026-08-27T10:00:00.000Z",
			"completedAt":"2026-08-27T10:04:00.000Z",
			"error":null,
			"createdAt":"2026-08-27T09:59:00.000Z",
			"updatedAt":"2026-08-27T10:04:00.000Z",
			"durationMs":240000
		},
		"hypotheses":[{
			"id":"hyp_1","investigationId":"inv_1",
			"description":"Total is summed as an integer and overflows above 1000 minor units",
			"rank":1,"status":"CONFIRMED",
			"supportingEvidenceIds":["ev_1","ev_2"],
			"contradictingEvidenceIds":["ev_3"],
			"createdAt":"2026-08-27T10:01:00.000Z","updatedAt":"2026-08-27T10:03:00.000Z"
		}],
		"patches":[],
		"verifications":[],
		"evidenceCount":7,
		"latestSequence":58
	}`)

	out, err := c.GetInvestigation(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("GetInvestigation: %v", err)
	}
	if out.Investigation.State != "REPRODUCED_AND_DIAGNOSED" {
		t.Errorf("State = %q", out.Investigation.State)
	}
	if out.Investigation.Outcome == nil || *out.Investigation.Outcome != "REPRODUCED_AND_DIAGNOSED" {
		t.Errorf("Outcome = %v", out.Investigation.Outcome)
	}
	if out.Investigation.DurationMs == nil || *out.Investigation.DurationMs != 240000 {
		t.Errorf("DurationMs = %v", out.Investigation.DurationMs)
	}
	if out.Investigation.Error != nil {
		t.Errorf("Error = %v, want nil for a run that did not break", out.Investigation.Error)
	}
	// The budget is an open bag and must survive as raw JSON rather than being
	// flattened into fields nobody agreed on.
	var budget map[string]any
	if err := json.Unmarshal(out.Investigation.Budget, &budget); err != nil {
		t.Fatalf("budget did not survive as JSON: %v", err)
	}
	if budget["spentUsd"] != 0.41 {
		t.Errorf("budget = %v", budget)
	}
	if out.EvidenceCount != 7 || out.LatestSequence != 58 {
		t.Errorf("counts = %d evidence, sequence %d", out.EvidenceCount, out.LatestSequence)
	}

	// BOTH sides of a hypothesis travel. A hypothesis rendered without what
	// contradicts it is an assertion.
	h := out.Hypotheses[0]
	if len(h.SupportingEvidenceIDs) != 2 || len(h.ContradictingEvidenceIDs) != 1 {
		t.Errorf("evidence lists = %v / %v", h.SupportingEvidenceIDs, h.ContradictingEvidenceIDs)
	}
}

// An investigation is created in state CREATED and nothing has run yet. The
// response must not be read as a finished run, and this pins that the empty
// halves stay empty rather than decoding into something that looks like a
// result.
func TestCreateInvestigationReturnsAnUnstartedRun(t *testing.T) {
	c := serveJSON(t, `{
		"investigation":{
			"id":"inv_new","orgId":"org_1","repositoryId":"repo_1",
			"issueRef":null,"issueTitle":"t","issueBody":"b",
			"state":"CREATED","outcome":null,"providerId":null,
			"startedAt":null,"completedAt":null,"error":null,
			"createdAt":"2026-08-27T12:00:00.000Z","updatedAt":"2026-08-27T12:00:00.000Z",
			"durationMs":null
		},
		"hypotheses":[],"patches":[],"verifications":[],
		"evidenceCount":0,"latestSequence":0
	}`)

	out, err := c.CreateInvestigation(context.Background(), CreateInvestigationInput{
		RepositoryID: "repo_1", IssueTitle: "t", IssueBody: "b",
	})
	if err != nil {
		t.Fatalf("CreateInvestigation: %v", err)
	}
	if out.Investigation.State != "CREATED" {
		t.Errorf("State = %q, want CREATED", out.Investigation.State)
	}
	if out.Investigation.Outcome != nil {
		t.Errorf("Outcome = %v; an unstarted run has concluded nothing", *out.Investigation.Outcome)
	}
	if out.Investigation.StartedAt != nil || out.Investigation.DurationMs != nil {
		t.Error("an unstarted run reported a start time or a duration")
	}
	if len(out.Patches) != 0 || len(out.Verifications) != 0 {
		t.Error("an unstarted run reported patches or verifications")
	}
}

// The events cursor. The engine cuts the page first and filters debug events
// after, so NextSince comes from the page and HasMore is about the page — an
// empty Events with HasMore true means "that page was all debug", not "the
// timeline ended". A client that stopped on an empty page would silently
// truncate a live timeline.
func TestEventPageCursorSurvivesADebugOnlyPage(t *testing.T) {
	c := serveJSON(t, `{
		"events":[],
		"latestSequence":300,
		"nextSince":200,
		"hasMore":true
	}`)

	page, err := c.InvestigationEvents(context.Background(), "inv_1", &EventQuery{Since: Int(100)})
	if err != nil {
		t.Fatalf("InvestigationEvents: %v", err)
	}
	if len(page.Events) != 0 {
		t.Fatalf("events = %d, want 0", len(page.Events))
	}
	if !page.HasMore {
		t.Error("HasMore was lost; a reader would stop mid-timeline")
	}
	if page.NextSince != 200 {
		t.Errorf("NextSince = %d, want 200 (the cursor comes from the page, not the survivors)", page.NextSince)
	}
	if page.LatestSequence != 300 {
		t.Errorf("LatestSequence = %d", page.LatestSequence)
	}
}

func TestInvestigationEvidenceDecodesASignature(t *testing.T) {
	c := serveJSON(t, `{
		"evidence":[{
			"id":"ev_1","investigationId":"inv_1",
			"type":"REPRODUCTION","phase":"BEFORE_PATCH","strength":"STRONG",
			"summary":"POST /checkout with total 1500 returns 500",
			"contentRef":"sha256:aa/bb","metadata":{"attempt":1},
			"signature":{
				"command":"npm test -- checkout.spec.ts",
				"exitCode":1,
				"errorClass":"AssertionError",
				"normalizedMessage":"expected 200, got 500",
				"originFile":"src/checkout/total.ts",
				"originLine":88,
				"failingTestIds":["checkout > over 1000"],
				"hash":"h_9f"
			},
			"createdAt":"2026-08-27T10:02:00.000Z"
		}],
		"total":1
	}`)

	page, err := c.InvestigationEvidence(context.Background(), "inv_1", nil)
	if err != nil {
		t.Fatalf("InvestigationEvidence: %v", err)
	}
	e := page.Evidence[0]
	if e.Signature == nil {
		t.Fatal("the captured signature was dropped; it is the proof")
	}
	if e.Signature.Command != "npm test -- checkout.spec.ts" {
		t.Errorf("Command = %q", e.Signature.Command)
	}
	if e.Signature.ExitCode == nil || *e.Signature.ExitCode != 1 {
		t.Errorf("ExitCode = %v", e.Signature.ExitCode)
	}
	if e.Signature.OriginLine == nil || *e.Signature.OriginLine != 88 {
		t.Errorf("OriginLine = %v", e.Signature.OriginLine)
	}
	if e.Signature.Hash != "h_9f" {
		t.Errorf("Hash = %q", e.Signature.Hash)
	}
	// Strength is an ordinal label. It must arrive as the label, never as a
	// number a renderer could turn into a percentage.
	if e.Strength != "STRONG" {
		t.Errorf("Strength = %q", e.Strength)
	}
}

// Local checkouts are served as `local:<name>`, never as a host path. A client
// that treated Source as cloneable would try to clone a label; more to the
// point, this pins that the client does not expect a path it was never sent.
func TestRepositorySourceCarriesTheLocalLabelUntouched(t *testing.T) {
	c := serveJSON(t, `{
		"repositories":[
			{"id":"r1","orgId":"o1","name":"web","source":"local:web","defaultBranch":"main","createdAt":"2026-01-01T00:00:00.000Z"},
			{"id":"r2","orgId":"o1","name":"api","source":"git@github.com:Credda-io/core.git","defaultBranch":"main","createdAt":"2026-01-01T00:00:00.000Z"}
		],
		"total":2
	}`)

	out, err := c.ListRepositories(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if out.Repositories[0].Source != "local:web" {
		t.Errorf("Source = %q, want the label verbatim", out.Repositories[0].Source)
	}
	if out.Repositories[1].Source != "git@github.com:Credda-io/core.git" {
		t.Errorf("Source = %q, want the clone URL verbatim", out.Repositories[1].Source)
	}
}

// A repository Credda has learned nothing about is an empty list with total 0
// and NO error. "We know nothing here" is a real answer, and a client that
// turned it into a failure would hide it.
func TestRepositoryLearningsEmptyIsNotAnError(t *testing.T) {
	c := serveJSON(t, `{"learnings":[],"total":0}`)
	page, err := c.RepositoryLearnings(context.Background(), "repo_1", nil)
	if err != nil {
		t.Fatalf("RepositoryLearnings: %v", err)
	}
	if page.Total != 0 || len(page.Learnings) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestRepositoryLearningsDecodesAnAnchoredLearning(t *testing.T) {
	c := serveJSON(t, `{
		"learnings":[{
			"id":"l_1","kind":"FRAGILE_SITE",
			"summary":"total.ts sums minor units as a 32-bit int",
			"filePath":"src/checkout/total.ts","symbol":"sumLineItems",
			"observations":3,"weight":"STRONG",
			"investigationIds":["inv_1","inv_7","inv_9"],
			"createdAt":"2026-06-01T00:00:00.000Z","lastSeenAt":"2026-08-27T10:03:00.000Z"
		},{
			"id":"l_2","kind":"CONVENTION",
			"summary":"Tests are run with pnpm, never npm",
			"filePath":null,"symbol":null,
			"observations":1,"weight":"WEAK",
			"investigationIds":["inv_2"],
			"createdAt":"2026-06-02T00:00:00.000Z","lastSeenAt":"2026-06-02T00:00:00.000Z"
		}],
		"total":2
	}`)

	page, err := c.RepositoryLearnings(context.Background(), "repo_1", &LearningQuery{})
	if err != nil {
		t.Fatalf("RepositoryLearnings: %v", err)
	}
	if page.Learnings[0].FilePath == nil || *page.Learnings[0].FilePath != "src/checkout/total.ts" {
		t.Errorf("FilePath = %v", page.Learnings[0].FilePath)
	}
	// An unanchored learning has no file, and nil must not decode to "".
	if page.Learnings[1].FilePath != nil {
		t.Errorf("FilePath = %q, want nil for an unanchored learning", *page.Learnings[1].FilePath)
	}
	if page.Learnings[0].Observations != 3 {
		t.Errorf("Observations = %d", page.Learnings[0].Observations)
	}
}

// ── cancellation ────────────────────────────────────────────────────────────
//
// The route reports what it ACHIEVED, and these tests exist because the obvious
// client is wrong: a nil error from CancelInvestigation does not mean the run
// stopped. Only Status says that.

// A 202 is a success at the transport level and MUST NOT be readable as
// "cancelled": a worker is still inside the run, holding a sandbox and possibly
// spending a model budget, and the API has written no terminal state. The
// state that comes back is the state the run is STILL IN.
func TestCancelInvestigationDoesNotReport202AsStopped(t *testing.T) {
	c, _ := newTestServer(t, 202, `{"investigationId":"inv_1","state":"ATTEMPTING_REPRODUCTION","status":"CANCELLATION_REQUESTED"}`)

	got, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{})
	if err != nil {
		t.Fatalf("CancelInvestigation: %v", err)
	}
	if got.Status != StatusCancellationRequested {
		t.Errorf("Status = %q, want %q", got.Status, StatusCancellationRequested)
	}
	if got.Stopped() {
		t.Error("Stopped() = true for CANCELLATION_REQUESTED; a worker is still inside the run")
	}
	if got.State == "CANCELLED" {
		t.Errorf("State = %q; the API writes no terminal state on 202, the run writes its own", got.State)
	}
	if got.State != "ATTEMPTING_REPRODUCTION" {
		t.Errorf("State = %q, want the state the run is still in", got.State)
	}
}

func TestCancelInvestigationReportsAQueuedRunAsStopped(t *testing.T) {
	c, _ := newTestServer(t, 200, `{"investigationId":"inv_1","state":"CANCELLED","status":"CANCELLED"}`)

	got, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{Reason: "wrong repository"})
	if err != nil {
		t.Fatalf("CancelInvestigation: %v", err)
	}
	if got.Status != StatusCancelled || !got.Stopped() {
		t.Errorf("Status = %q, Stopped() = %v; want CANCELLED and true", got.Status, got.Stopped())
	}
	if got.InvestigationID != "inv_1" || got.State != "CANCELLED" {
		t.Errorf("got %+v", got)
	}
}

// Repeating the call is not an error, and is still stopped.
func TestCancelInvestigationIsIdempotent(t *testing.T) {
	c, _ := newTestServer(t, 200, `{"investigationId":"inv_1","state":"CANCELLED","status":"ALREADY_CANCELLED"}`)

	got, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{})
	if err != nil {
		t.Fatalf("CancelInvestigation: %v", err)
	}
	if got.Status != StatusAlreadyCancelled || !got.Stopped() {
		t.Errorf("Status = %q, Stopped() = %v; want ALREADY_CANCELLED and true", got.Status, got.Stopped())
	}
}

// The two refusals are errors, because neither stopped anything. Their codes
// are the whole message: one says there is nothing left to stop, the other says
// this API cannot reach what is running.
func TestCancelInvestigationSurfacesTheTwoRefusals(t *testing.T) {
	for _, tc := range []struct {
		code string
		body string
	}{
		{CodeAlreadyFinished, `{"error":{"code":"ALREADY_FINISHED","message":"Investigation inv_1 already finished in VERIFIED"}}`},
		{CodeNotCancellable, `{"error":{"code":"NOT_CANCELLABLE","message":"Investigation inv_1 is running outside the job queue and cannot be stopped through this API"}}`},
	} {
		t.Run(tc.code, func(t *testing.T) {
			c, _ := newTestServer(t, 409, tc.body)
			got, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{})
			if got != nil {
				t.Errorf("got = %+v, want nil", got)
			}
			apiErr, ok := AsAPIError(err)
			if !ok {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.StatusCode != 409 || apiErr.Code != tc.code {
				t.Errorf("status = %d, code = %q; want 409 and %q", apiErr.StatusCode, apiErr.Code, tc.code)
			}
		})
	}
}

// Never repeated, whatever WithRetries says: see the doc comment on
// CancelInvestigation.
func TestCancelInvestigationIsNotRetried(t *testing.T) {
	c, got := newTestServer(t, 503, `{"error":{"code":"UNAVAILABLE","message":"down"}}`)
	c.retries = 3
	c.retryBase = 0

	if _, err := c.CancelInvestigation(context.Background(), "inv_1", CancelInvestigationInput{}); err == nil {
		t.Fatal("want an error")
	}
	if got.Calls != 1 {
		t.Errorf("calls = %d, want 1", got.Calls)
	}
}
