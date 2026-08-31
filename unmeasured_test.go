package credda

import (
	"context"
	"strings"
	"testing"
)

// UNMEASURED IS NOT ZERO: the Go half of the rule.
//
// encoding/json decodes a JSON null into a non-pointer field as a NO-OP: the
// field keeps its zero value and no error is returned. Nothing fails, nothing
// warns, and a value the engine deliberately did not measure arrives as a
// confident number.
//
// The engine is careful about this in a way this client must not undo. It
// writes, in as many words: a resolution's fix is null exactly when the run
// produced no such row; a verification verdict is null when no run exists and
// is "never a stand-in verdict"; timedOutAttempts is "served as null rather
// than as zero, because the record does not say what happened to a run written
// before the column existed, and a consumer that read zero would take that as
// 'nothing was killed'". Every one of those becomes a lie in a plain int.
//
// So every nullable field on the wire is a POINTER here, the absence survives
// decoding as nil, and the compiler forces a reader to say what nil means.
// These tests pin the three shapes that matter:
//
//  1. an absent value stays absent after decoding;
//  2. a MEASURED zero stays distinguishable from an unmeasured one, and stays
//     displayable, because a real 0 is a real answer;
//  3. a payload from an OLDER engine that omits the field decodes safely.

// ── 1. absence survives ─────────────────────────────────────────────────────

// A run that has not finished has no duration, no outcome and no completion
// time. Decoded into plain values these would be a 0ms run that completed at
// the zero time with an empty outcome — a finished, instantaneous, unlabelled
// investigation that never happened.
func TestUnfinishedRunHasNoDurationAndNoOutcome(t *testing.T) {
	c := serveJSON(t, `{
		"investigations":[{
			"id":"inv_running","issueRef":null,"issueTitle":"Checkout 500s",
			"state":"ATTEMPTING_REPRODUCTION","outcome":null,"providerId":null,
			"startedAt":"2026-08-27T10:00:00.000Z","completedAt":null,
			"createdAt":"2026-08-27T09:59:00.000Z","durationMs":null,
			"eventCount":12,"evidenceCount":3
		}],
		"total":1
	}`)

	out, err := c.ListInvestigations(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListInvestigations: %v", err)
	}
	row := out.Investigations[0]
	if row.Outcome != nil {
		t.Errorf("Outcome = %q; a running investigation has concluded nothing", *row.Outcome)
	}
	if row.CompletedAt != nil {
		t.Errorf("CompletedAt = %q; the run has not completed", *row.CompletedAt)
	}
	if row.DurationMs != nil {
		t.Errorf("DurationMs = %d; nil means the run has not both started and finished", *row.DurationMs)
	}
	// The counts around it are real and must not be dragged into nil-ness.
	if row.EventCount != 12 || row.EvidenceCount != 3 {
		t.Errorf("counts = %d/%d", row.EventCount, row.EvidenceCount)
	}
}

// The engine's own example: null and 0 on the same field, on two records, with
// two different meanings. Nil says "this record does not say"; 0 says "nothing
// was killed". Collapsing them loses the second.
func TestNullAndZeroTimedOutAttemptsStayDifferent(t *testing.T) {
	body := func(v string) string {
		return `{"resolution":{
			"id":"res_x","investigationId":"inv_x",
			"bug":{"reported":"x","reference":null,"signalId":null,"affectedFiles":[]},
			"evidence":[],
			"reproduction":{"status":"REPRODUCED","command":null,"signature":null,
			                "evidenceId":null,"timedOutAttempts":` + v + `},
			"rootCause":null,"fix":null,"verification":null,
			"regressionProtection":{"status":"UNPROTECTED","before":"NOT_RUN","after":"NOT_RUN"},
			"confidence":{"class":"NOT_ESTABLISHED","notEstablished":["n"]},
			"createdAt":"2026-08-27T00:00:00.000Z"
		}}`
	}

	unwritten, err := serveJSON(t, body("null")).GetResolution(context.Background(), "res_x")
	if err != nil {
		t.Fatalf("GetResolution: %v", err)
	}
	measuredZero, err := serveJSON(t, body("0")).GetResolution(context.Background(), "res_x")
	if err != nil {
		t.Fatalf("GetResolution: %v", err)
	}

	if unwritten.Reproduction.TimedOutAttempts != nil {
		t.Errorf("a null timedOutAttempts decoded to %d", *unwritten.Reproduction.TimedOutAttempts)
	}
	if measuredZero.Reproduction.TimedOutAttempts == nil {
		t.Fatal("a MEASURED zero decoded to nil; 'nothing was killed' is a real answer and was erased")
	}
	if *measuredZero.Reproduction.TimedOutAttempts != 0 {
		t.Errorf("measured zero = %d", *measuredZero.Reproduction.TimedOutAttempts)
	}
}

// ── 2. a measured zero is displayable ───────────────────────────────────────

// ExecutableFilesChanged 0 is the NO_CHANGE_REQUIRED outcome: the change was
// analysed and touched nothing executable, which is a SUCCESS. Null means the
// change was never analysed at all. A queue that renders both as "0 files"
// tells a reviewer a run succeeded when it may never have started.
func TestExecutableFilesChangedZeroIsNotTheSameAsUnanalysed(t *testing.T) {
	c := serveJSON(t, `{
		"validations":[
			{"id":"val_none","repositoryId":"r1","repositorySource":"local:web",
			 "sourceType":"BRANCH","sourceRef":"docs/readme","baseCommit":"a","headCommit":"b",
			 "state":"COMPLETED","outcome":"NO_CHANGE_REQUIRED","triggerKind":"MANUAL",
			 "environmentStatus":"NOT_REQUIRED","environmentFailureKind":null,
			 "executableFilesChanged":0,
			 "startedAt":"2026-08-27T10:00:00.000Z","completedAt":"2026-08-27T10:00:05.000Z",
			 "createdAt":"2026-08-27T10:00:00.000Z","durationMs":5000},
			{"id":"val_new","repositoryId":"r1","repositorySource":"local:web",
			 "sourceType":"BRANCH","sourceRef":"feat/x","baseCommit":null,"headCommit":null,
			 "state":"CREATED","outcome":null,"triggerKind":"MANUAL",
			 "environmentStatus":"PENDING","environmentFailureKind":null,
			 "executableFilesChanged":null,
			 "startedAt":null,"completedAt":null,
			 "createdAt":"2026-08-27T10:01:00.000Z","durationMs":null}
		],
		"total":2
	}`)

	out, err := c.ListValidations(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListValidations: %v", err)
	}
	analysed, unanalysed := out.Validations[0], out.Validations[1]

	if analysed.ExecutableFilesChanged == nil {
		t.Fatal("a measured 0 decoded to nil; NO_CHANGE_REQUIRED is a success and was erased")
	}
	if *analysed.ExecutableFilesChanged != 0 {
		t.Errorf("ExecutableFilesChanged = %d, want 0", *analysed.ExecutableFilesChanged)
	}
	if unanalysed.ExecutableFilesChanged != nil {
		t.Errorf("an unanalysed change reported %d files", *unanalysed.ExecutableFilesChanged)
	}
	if unanalysed.Outcome != nil {
		t.Error("a CREATED validation reported an outcome")
	}
}

// A completed run with zero checks is the false success this product exists to
// prevent. The count is a real, measured 0 and must reach a caller as one.
func TestValidationCheckCountZeroIsVisible(t *testing.T) {
	c := serveJSON(t, `{
		"validation":{"id":"val_1","orgId":"o1","repositoryId":"r1",
			"sourceType":"BRANCH","sourceRef":"feat/x","baseCommit":"a","headCommit":"b",
			"state":"COMPLETED","outcome":"BLOCKED","triggerKind":"MANUAL","triggeredBy":null,
			"intentSummary":null,"executableFilesChanged":4,
			"startedAt":"2026-08-27T10:00:00.000Z","completedAt":"2026-08-27T10:02:00.000Z",
			"error":null,"createdAt":"2026-08-27T09:59:00.000Z","updatedAt":"2026-08-27T10:02:00.000Z",
			"durationMs":120000},
		"environment":{"status":"FAILED","failureKind":"DEPENDENCY_INSTALL",
			"detail":{"runtime":"node@22","command":"pnpm install --frozen-lockfile",
			          "stderr":"ERR_PNPM_OUTDATED_LOCKFILE"}},
		"changeImpact":{"executableFiles":4},
		"checkCount":0,"findingCount":0,"evidenceCount":0,"latestSequence":11
	}`)

	out, err := c.GetValidation(context.Background(), "val_1")
	if err != nil {
		t.Fatalf("GetValidation: %v", err)
	}
	if out.CheckCount != 0 {
		t.Errorf("CheckCount = %d", out.CheckCount)
	}
	// BLOCKED is an environment failure, not a product failure. Both the
	// outcome and the reason must be readable without a second request.
	if out.Validation.Outcome == nil || *out.Validation.Outcome != "BLOCKED" {
		t.Errorf("Outcome = %v, want BLOCKED", out.Validation.Outcome)
	}
	if out.Environment.FailureKind == nil || *out.Environment.FailureKind != "DEPENDENCY_INSTALL" {
		t.Errorf("FailureKind = %v", out.Environment.FailureKind)
	}
	// The detail is what an operator acts on. Summarising it away is what makes
	// a BLOCKED report useless, so it must arrive whole.
	if len(out.Environment.Detail) == 0 {
		t.Fatal("the environment detail was dropped")
	}
	if !strings.Contains(string(out.Environment.Detail), "ERR_PNPM_OUTDATED_LOCKFILE") {
		t.Errorf("the failing stderr did not survive: %s", out.Environment.Detail)
	}
}

// BaseStatus is the whole product on one field. A check that FAILED with
// BaseStatus PASSED means this change caused it; a nil BaseStatus means the
// base was never re-run, and reading nil as "passed" would report a
// pre-existing failure as a regression this change introduced.
func TestCheckBaseStatusDistinguishesCausedFromPreexisting(t *testing.T) {
	c := serveJSON(t, `{
		"checks":[
			{"id":"c1","validationId":"v1","sequence":1,"name":"checkout over 1000",
			 "category":"FUNCTIONAL","method":"TEST","reason":"the change touches total.ts",
			 "target":null,"expectedBehavior":null,"requirementSource":null,
			 "status":"FAILED","baseStatus":"PASSED","investigationId":"inv_1",
			 "startedAt":null,"completedAt":null,"detail":null,
			 "createdAt":"2026-08-27T10:00:00.000Z","durationMs":null},
			{"id":"c2","validationId":"v1","sequence":2,"name":"flaky upload",
			 "category":"INTEGRATION","method":"TEST","reason":"touched",
			 "target":null,"expectedBehavior":null,"requirementSource":null,
			 "status":"FAILED","baseStatus":"FAILED","investigationId":null,
			 "startedAt":null,"completedAt":null,"detail":null,
			 "createdAt":"2026-08-27T10:00:00.000Z","durationMs":null},
			{"id":"c3","validationId":"v1","sequence":3,"name":"never re-run",
			 "category":"BUILD","method":"COMMAND","reason":"touched",
			 "target":null,"expectedBehavior":null,"requirementSource":null,
			 "status":"FAILED","baseStatus":null,"investigationId":null,
			 "startedAt":null,"completedAt":null,"detail":null,
			 "createdAt":"2026-08-27T10:00:00.000Z","durationMs":null}
		],
		"total":3
	}`)

	page, err := c.ValidationChecks(context.Background(), "v1", nil)
	if err != nil {
		t.Fatalf("ValidationChecks: %v", err)
	}
	caused, preexisting, unknown := page.Checks[0], page.Checks[1], page.Checks[2]

	if caused.BaseStatus == nil || *caused.BaseStatus != "PASSED" {
		t.Errorf("caused.BaseStatus = %v, want PASSED", caused.BaseStatus)
	}
	if preexisting.BaseStatus == nil || *preexisting.BaseStatus != "FAILED" {
		t.Errorf("preexisting.BaseStatus = %v, want FAILED", preexisting.BaseStatus)
	}
	if unknown.BaseStatus != nil {
		t.Errorf("a check whose base was never re-run reported %q", *unknown.BaseStatus)
	}
	if unknown.InvestigationID != nil {
		t.Error("a null investigationId decoded to a non-nil value")
	}
}

// ── 3. an older engine's payload decodes safely ─────────────────────────────

// A field the engine has not started sending yet must leave nil behind, not a
// decode error and not a fabricated zero. This is the shape a client sees while
// a deployment lags the SDK.
func TestOmittedFieldsDecodeToNil(t *testing.T) {
	c := serveJSON(t, `{
		"investigations":[{"id":"inv_old","issueTitle":"legacy","state":"COMPLETED",
		                   "createdAt":"2026-01-01T00:00:00.000Z"}],
		"total":1
	}`)

	out, err := c.ListInvestigations(context.Background(), nil)
	if err != nil {
		t.Fatalf("a payload missing newer fields failed to decode: %v", err)
	}
	row := out.Investigations[0]
	if row.ID != "inv_old" || row.State != "COMPLETED" {
		t.Errorf("the present fields were lost: %+v", row)
	}
	for name, got := range map[string]any{
		"IssueRef":    row.IssueRef,
		"Outcome":     row.Outcome,
		"ProviderID":  row.ProviderID,
		"StartedAt":   row.StartedAt,
		"CompletedAt": row.CompletedAt,
		"DurationMs":  row.DurationMs,
	} {
		if !isNilPtr(got) {
			t.Errorf("%s was omitted by the server but decoded to %v", name, got)
		}
	}
}

// An unknown field on the wire is not an error: the engine may serve a value
// this client predates, and refusing the whole record over it would break every
// caller on the day a field is added.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	c := serveJSON(t, `{
		"repositories":[{"id":"r1","orgId":"o1","name":"web","source":"local:web",
		                 "defaultBranch":"main","createdAt":"2026-01-01T00:00:00.000Z",
		                 "somethingNewer":{"a":1}}],
		"total":1,
		"alsoNewer":true
	}`)

	out, err := c.ListRepositories(context.Background(), nil)
	if err != nil {
		t.Fatalf("an unknown field broke decoding: %v", err)
	}
	if out.Repositories[0].Name != "web" {
		t.Errorf("got %+v", out.Repositories[0])
	}
}

// ── the member role pair ────────────────────────────────────────────────────

// Role and RoleEnforced are one fact written twice. RoleEnforced is false
// because api_keys names an organisation and never a person, so there is no
// member on a request for a role to be looked up for. A client that decoded
// Role and dropped RoleEnforced would hand a UI a rendered access model this
// product does not have.
func TestMemberRoleArrivesWithItsEnforcementFlag(t *testing.T) {
	c := serveJSON(t, `{
		"members":[{"userId":"u1","email":"ana@example.com","name":"Ana Ruiz",
		            "avatar":{"kind":"INITIALS","initials":"AR","colorIndex":3},
		            "role":"OWNER","roleEnforced":false,"joinedAt":"2026-01-01T00:00:00.000Z"}],
		"total":1
	}`)

	page, err := c.OrganizationMembers(context.Background(), nil)
	if err != nil {
		t.Fatalf("OrganizationMembers: %v", err)
	}
	m := page.Members[0]
	if m.Role != "OWNER" {
		t.Errorf("Role = %q", m.Role)
	}
	if m.RoleEnforced {
		t.Error("RoleEnforced decoded true; nothing in the engine enforces a role")
	}
	if m.Avatar.Initials != "AR" || m.Avatar.ColorIndex != 3 {
		t.Errorf("Avatar = %+v", m.Avatar)
	}
}

// An unused key has never been used; nil is that, and a zero timestamp is not.
// A revoked key keeps its revocation date, which is the answer an operator came
// for.
func TestAPIKeyTimestampsKeepTheirAbsence(t *testing.T) {
	c := serveJSON(t, `{
		"keys":[
			{"id":"key_live","name":"ci","createdAt":"2026-01-01T00:00:00.000Z",
			 "lastUsedAt":"2026-08-27T09:00:00.000Z","revokedAt":null},
			{"id":"key_never","name":"spare","createdAt":"2026-01-01T00:00:00.000Z",
			 "lastUsedAt":null,"revokedAt":null},
			{"id":"key_gone","name":"leaked","createdAt":"2026-01-01T00:00:00.000Z",
			 "lastUsedAt":"2026-03-01T00:00:00.000Z","revokedAt":"2026-03-03T00:00:00.000Z"}
		],
		"total":3
	}`)

	page, err := c.OrganizationKeys(context.Background(), nil)
	if err != nil {
		t.Fatalf("OrganizationKeys: %v", err)
	}
	if page.Keys[1].LastUsedAt != nil {
		t.Errorf("a never-used key reported lastUsedAt %q", *page.Keys[1].LastUsedAt)
	}
	if page.Keys[0].RevokedAt != nil {
		t.Error("a live key reported a revocation date")
	}
	if page.Keys[2].RevokedAt == nil || *page.Keys[2].RevokedAt != "2026-03-03T00:00:00.000Z" {
		t.Errorf("RevokedAt = %v; 'this key was revoked on the 3rd' is the answer", page.Keys[2].RevokedAt)
	}
}

// A schema version that could not be read at all is nil, not 0. Version 0 would
// read as "an unmigrated database", which is a different failure from "the
// version is unreadable".
func TestUnreadableSchemaVersionIsNilNotZero(t *testing.T) {
	c := serveJSON(t, `{
		"status":"ok","schemaVersion":null,"expectedSchemaVersion":42,
		"checks":[{"name":"migrations","status":"unknown","detail":"version unreadable: SQLITE_BUSY"}]
	}`)

	out, err := c.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if out.SchemaVersion != nil {
		t.Errorf("SchemaVersion = %d; an unreadable version is not version 0", *out.SchemaVersion)
	}
	if out.ExpectedSchemaVersion != 42 {
		t.Errorf("ExpectedSchemaVersion = %d", out.ExpectedSchemaVersion)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func isNilPtr(v any) bool {
	switch p := v.(type) {
	case *string:
		return p == nil
	case *int:
		return p == nil
	case *int64:
		return p == nil
	case *bool:
		return p == nil
	default:
		return v == nil
	}
}

// ── 4. fields the engine sends that this client used to drop ────────────────
//
// The other direction of the same defect. A field the API sends and no struct
// names is discarded by encoding/json without a word, so the failure is not a
// wrong value on screen but a value that never arrives — and every one of these
// was on the wire while this package's own doc comments said it was not there.
// They are pinned as decodes rather than described.

// The queue row carries its repository and its signal. This type used to say
// "it deliberately carries no RepositoryID: the engine's list serializer omits
// it", which sent a queue screen down one detail request per row for a field
// that was already in the page and was being thrown away here.
func TestQueueRowCarriesItsRepositoryAndSignal(t *testing.T) {
	c := serveJSON(t, `{
		"investigations":[{"id":"inv_1","repositoryId":"repo_1",
		                   "repositorySource":"local:web","issueRef":null,
		                   "issueTitle":"Checkout 500s","signalId":"sig_9",
		                   "state":"READY_FOR_REVIEW","outcome":"VERIFIED",
		                   "providerId":"claude","startedAt":null,"completedAt":null,
		                   "createdAt":"2026-08-30T00:00:00.000Z","durationMs":null,
		                   "eventCount":3,"evidenceCount":2}],
		"total":1
	}`)

	out, err := c.ListInvestigations(context.Background(), nil)
	if err != nil {
		t.Fatalf("decoding the queue: %v", err)
	}
	row := out.Investigations[0]
	if row.RepositoryID != "repo_1" {
		t.Errorf("RepositoryID = %q, want repo_1", row.RepositoryID)
	}
	if row.RepositorySource == nil || *row.RepositorySource != "local:web" {
		t.Errorf("RepositorySource = %v, want local:web", row.RepositorySource)
	}
	if row.SignalID == nil || *row.SignalID != "sig_9" {
		t.Errorf("SignalID = %v, want sig_9", row.SignalID)
	}
	// The patch path is on the engine's investigation path since ADR 0019, and
	// this vocabulary withheld both of these until it was corrected.
	if row.State != "READY_FOR_REVIEW" || row.Outcome == nil || *row.Outcome != "VERIFIED" {
		t.Errorf("a patch-path row did not survive: %+v", row)
	}
}

// A signal set with no signal record and a run nothing raised are DIFFERENT
// answers, and only both fields together separate them: the engine's signal
// read is organisation-scoped, so a run pointed at another organisation's
// signal arrives with SignalID set and Signal nil. Reading that as "nobody
// reported this" is a claim about the record made from a fact about the caller.
func TestSignalNotVisibleIsNotSignalAbsent(t *testing.T) {
	c := serveJSON(t, `{
		"investigation":{"id":"inv_1","orgId":"o1","repositoryId":"r1","issueRef":null,
		                 "issueTitle":"t","issueBody":"b","signalId":"sig_other",
		                 "state":"INVESTIGATING","outcome":null,"providerId":null,
		                 "startedAt":null,"completedAt":null,"error":null,
		                 "createdAt":"2026-08-30T00:00:00.000Z",
		                 "updatedAt":"2026-08-30T00:00:00.000Z","durationMs":null},
		"signal":null,
		"hypotheses":[],"patches":[],"verifications":[],
		"cost":null,"effectiveBudget":null,
		"evidenceCount":0,"latestSequence":0
	}`)

	out, err := c.GetInvestigation(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("decoding the detail: %v", err)
	}
	if out.Investigation.SignalID == nil {
		t.Fatal("SignalID was dropped; the two nil readings are now indistinguishable")
	}
	if out.Signal != nil {
		t.Errorf("Signal = %+v, want nil", out.Signal)
	}
	// Null is not zero, one field over: a run still executing recorded no cost
	// and was given no recorded ceiling.
	if out.Cost != nil || out.EffectiveBudget != nil {
		t.Errorf("an unrecorded spend decoded to a value: %+v %+v", out.Cost, out.EffectiveBudget)
	}
}

// What a finished run spent, and under what ceiling. modelCostBasis is not
// decoration: UNPRICED makes ModelCostUsd a floor, so both have to arrive.
func TestRunCostAndEffectiveBudgetDecode(t *testing.T) {
	c := serveJSON(t, `{
		"investigation":{"id":"inv_1","orgId":"o1","repositoryId":"r1","issueRef":null,
		                 "issueTitle":"t","issueBody":"b","signalId":null,
		                 "state":"READY_FOR_REVIEW","outcome":"VERIFIED","providerId":"claude",
		                 "startedAt":null,"completedAt":null,"error":null,
		                 "createdAt":"2026-08-30T00:00:00.000Z",
		                 "updatedAt":"2026-08-30T00:00:00.000Z","durationMs":null},
		"signal":null,"hypotheses":[],"patches":[],"verifications":[],
		"cost":{"investigationId":"inv_1","providerId":"claude","wallClockMs":65000,
		        "commandMs":1200,"commandCount":9,"sandbox":null,"modelCallCount":11,
		        "inputTokens":40000,"outputTokens":3000,"cachedInputTokens":0,
		        "modelCostUsd":0.1255,"modelCostBasis":"UNPRICED",
		        "recordedAt":"2026-08-30T00:01:05.000Z"},
		"effectiveBudget":{"investigationId":"inv_1",
		        "limits":{"maxWallClockMs":900000,"maxModelCalls":40,"maxTokens":400000,
		                  "maxToolCalls":120,"maxCommandMs":120000,"maxSandboxMs":900000,
		                  "maxPatchAttempts":3,"maxCostUsd":5},
		        "requestedFields":["maxCostUsd"],"clampedFields":[],"attemptsStarted":1,
		        "recordedAt":"2026-08-30T00:00:01.000Z",
		        "lastAttemptAt":"2026-08-30T00:00:01.000Z"},
		"evidenceCount":4,"latestSequence":22
	}`)

	out, err := c.GetInvestigation(context.Background(), "inv_1")
	if err != nil {
		t.Fatalf("decoding the detail: %v", err)
	}
	if out.Cost == nil || out.Cost.ModelCostBasis != "UNPRICED" {
		t.Fatalf("Cost = %+v; the basis that makes the figure a floor was dropped", out.Cost)
	}
	// The sandbox was never provisioned. Nil, not four zeroes claiming it was.
	if out.Cost.Sandbox != nil {
		t.Errorf("Sandbox = %+v, want nil", out.Cost.Sandbox)
	}
	if out.EffectiveBudget == nil || out.EffectiveBudget.Limits.MaxCostUsd != 5 {
		t.Fatalf("EffectiveBudget = %+v", out.EffectiveBudget)
	}
	if len(out.EffectiveBudget.RequestedFields) != 1 || out.EffectiveBudget.AttemptsStarted != 1 {
		t.Errorf("EffectiveBudget = %+v", out.EffectiveBudget)
	}
}

// The create route says what it did with the run, and this client always gets
// NOT_REQUESTED because CreateInvestigationInput sends no `start`. The field is
// read off the response rather than assumed, so the day this client learns to
// ask, nothing here has to change to notice.
func TestCreateResponseSaysWhetherARunWasQueued(t *testing.T) {
	c := serveJSON(t, `{
		"investigation":{"id":"inv_new","orgId":"o1","repositoryId":"r1","issueRef":null,
		                 "issueTitle":"t","issueBody":"b","signalId":null,"state":"CREATED",
		                 "outcome":null,"providerId":null,"startedAt":null,"completedAt":null,
		                 "error":null,"createdAt":"2026-08-30T00:00:00.000Z",
		                 "updatedAt":"2026-08-30T00:00:00.000Z","durationMs":null},
		"signal":null,"hypotheses":[],"patches":[],"verifications":[],
		"cost":null,"effectiveBudget":null,"evidenceCount":0,"latestSequence":0,
		"start":"NOT_REQUESTED","budget":null
	}`)

	out, err := c.CreateInvestigation(context.Background(), CreateInvestigationInput{
		RepositoryID: "r1", IssueTitle: "t", IssueBody: "b",
	})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if out.Start == nil || *out.Start != "NOT_REQUESTED" {
		t.Errorf("Start = %v, want NOT_REQUESTED — this client queues nothing", out.Start)
	}
	if out.RequestedBudget != nil {
		t.Errorf("RequestedBudget = %+v; a request that queued no job has no ceiling", out.RequestedBudget)
	}
}

// Null and empty are different answers: null is a record written before the
// engine kept refusals and says nothing, empty is a run that declined nothing.
// Collapsing them reports "Credda declined no part of this report" about a
// record that was never asked.
func TestDeclinedReproductionsNullIsNotEmpty(t *testing.T) {
	const body = `{"resolution":{"id":"res_1","investigationId":"inv_1",
		"bug":{"reported":"x","reference":null,"signalId":null,"affectedFiles":[]},
		"evidence":[],
		"reproduction":{"status":"REPRODUCED","command":null,"signature":null,
		                "evidenceId":null,"timedOutAttempts":null},
		"rootCause":null,"fix":null,"verification":null,
		"regressionProtection":{"status":"NONE","before":"NOT_RUN","after":"NOT_RUN"},
		%s
		"confidence":{"class":"NOT_ESTABLISHED","notEstablished":["nothing"]},
		"createdAt":"2026-08-30T00:00:00.000Z"}}`

	silent := serveJSON(t, strings.Replace(body, "%s", `"declinedReproductions":null,`, 1))
	r, err := silent.GetResolution(context.Background(), "res_1")
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if r.DeclinedReproductions != nil {
		t.Errorf("a record that says nothing decoded to %v", *r.DeclinedReproductions)
	}

	declined := serveJSON(t, strings.Replace(body, "%s",
		`"declinedReproductions":[{"source":"issue","reason":"asks for a credential","excerpt":"log in as admin"}],`, 1))
	r2, err := declined.GetResolution(context.Background(), "res_1")
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if r2.DeclinedReproductions == nil || len(*r2.DeclinedReproductions) != 1 {
		t.Fatalf("DeclinedReproductions = %v", r2.DeclinedReproductions)
	}
	if (*r2.DeclinedReproductions)[0].Reason != "asks for a credential" {
		t.Errorf("got %+v", (*r2.DeclinedReproductions)[0])
	}
}

// The workspace carries the ceiling a run started now would get, and the count
// of distinct reported failures work was opened from. Both were being dropped.
func TestOrganizationCarriesItsSignalCountAndDefaultBudget(t *testing.T) {
	c := serveJSON(t, `{
		"organization":{"id":"o1","name":"Acme","slug":"acme",
		                "createdAt":"2026-01-01T00:00:00.000Z"},
		"memberCount":0,"apiKeyCount":1,"revokedApiKeyCount":0,
		"repositoryCount":2,"investigationCount":9,"validationCount":3,
		"signalCount":4,
		"defaultRunBudget":{"maxWallClockMs":900000,"maxModelCalls":40,"maxTokens":400000,
		                    "maxToolCalls":120,"maxCommandMs":120000,"maxSandboxMs":900000,
		                    "maxPatchAttempts":3,"maxCostUsd":5}
	}`)

	out, err := c.GetOrganization(context.Background())
	if err != nil {
		t.Fatalf("decoding the workspace: %v", err)
	}
	if out.SignalCount != 4 {
		t.Errorf("SignalCount = %d, want 4", out.SignalCount)
	}
	if out.DefaultRunBudget == nil || out.DefaultRunBudget.MaxCostUsd != 5 {
		t.Fatalf("DefaultRunBudget = %+v", out.DefaultRunBudget)
	}
	// An engine that predates the field leaves nil rather than a budget of
	// zeroes, which would read as a run that may spend nothing.
	older := serveJSON(t, `{"organization":{"id":"o1","name":"Acme","slug":"acme",
		"createdAt":"2026-01-01T00:00:00.000Z"},"memberCount":0,"apiKeyCount":1,
		"revokedApiKeyCount":0,"repositoryCount":0,"investigationCount":0,
		"validationCount":0}`)
	out2, err := older.GetOrganization(context.Background())
	if err != nil {
		t.Fatalf("decoding an older workspace: %v", err)
	}
	if out2.DefaultRunBudget != nil {
		t.Errorf("DefaultRunBudget = %+v, want nil", out2.DefaultRunBudget)
	}
}

// A check runs inside an investigation, so an investigation's evidence page
// mixes rows the investigation produced with rows one of its checks did. The
// engine sends validationId and checkId together precisely so a client can tell
// them apart; this client named only one of the pair.
func TestInvestigationEvidenceSaysWhichCheckProducedIt(t *testing.T) {
	c := serveJSON(t, `{"evidence":[
		{"id":"ev_1","investigationId":"inv_1","validationId":null,"checkId":null,
		 "type":"REPRODUCTION","phase":"BEFORE_PATCH","strength":"STRONG","summary":"a",
		 "contentRef":null,"metadata":{},"signature":null,
		 "createdAt":"2026-08-30T00:00:00.000Z"},
		{"id":"ev_2","investigationId":"inv_1","validationId":"val_7","checkId":"chk_3",
		 "type":"TEST_RESULT","phase":"INDEPENDENT","strength":"MODERATE","summary":"b",
		 "contentRef":null,"metadata":{},"signature":null,
		 "createdAt":"2026-08-30T00:00:01.000Z"}],"total":2}`)

	out, err := c.InvestigationEvidence(context.Background(), "inv_1", nil)
	if err != nil {
		t.Fatalf("decoding evidence: %v", err)
	}
	if out.Evidence[0].ValidationID != nil {
		t.Errorf("the investigation's own row claimed a validation: %v", out.Evidence[0].ValidationID)
	}
	if out.Evidence[1].ValidationID == nil || *out.Evidence[1].ValidationID != "val_7" {
		t.Errorf("ValidationID = %v, want val_7", out.Evidence[1].ValidationID)
	}
}
