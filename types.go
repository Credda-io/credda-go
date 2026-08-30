package credda

import "encoding/json"

// This file mirrors apps/api/src/serialize.ts field for field. Every mapper
// there is written out explicitly so that a new database column cannot leak to
// a client silently; every struct here is written out explicitly for the mirror
// reason, so that a field appearing on the wire that nobody typed is visible as
// a field this client does not read.
//
// # Absence is a pointer, never a zero
//
// encoding/json decodes a JSON null into a non-pointer field as a no-op: the
// field keeps its zero value and no error is returned. That is fine for a
// counter and wrong for everything the engine deliberately leaves null.
//
// The engine leaves things null on purpose and says so in the serializer: a
// resolution's Fix is null exactly when the run produced no patch;
// VerificationVerdict is null when no verification run exists and is "never a
// stand-in verdict"; Reproduction.TimedOutAttempts is null rather than 0
// because a record written before the column existed does not say that nothing
// was killed. Decoded into a plain int, every one of those becomes a confident
// zero — a claim nothing measured.
//
// So every nullable field below is a pointer, and nil is the absence surviving
// the trip. Where the engine pairs a value with the reason it is absent —
// ConfidenceClass with NotEstablished, Role with RoleEnforced, State with
// Outcome — both halves are here, because the engine's rule is that neither can
// be read alone.
//
// # Open bags are json.RawMessage
//
// Metadata, Budget, ChangeImpact, Environment.Detail, ValidationEvent.Data and
// ValidationCheck.Detail are passed through by the engine exactly as it
// recorded them. serialize.ts says of the environment detail that "no field of
// it is an API contract yet", and the same holds for the others. Typing them
// here would be inventing a contract the engine has not made. They arrive as
// raw JSON for a caller to decode against whatever the engine wrote.

// ── vocabularies ────────────────────────────────────────────────────────────
//
// These are the exact `as const` tuples the API's zod schemas validate query
// parameters against. A value outside one of them is a 400 VALIDATION_FAILED.
//
// The states and outcomes a run can report are NOT enumerated as Go constants,
// because the API does not validate them on the way out and pinning them here
// would put this client in the business of rejecting a state the engine gained.
// They are strings. InvestigationStates and InvestigationOutcomes below are the
// vocabulary as of the API this client was written against; read them, do not
// switch exhaustively on them.

// InvestigationStates is the state filter vocabulary for ListInvestigations,
// from INVESTIGATION_STATES in packages/shared/src/states.ts.
//
// Absent from this list, and absent from the engine's own: the patch-path
// states. They are withheld pending a model-backed run, and a state the product
// cannot currently reach must not appear in a list of states the product has.
// See ADR 0018: that withholding is a status with a date on it, not a principle,
// and it moves when the number moves.
var InvestigationStates = []string{
	"CREATED",
	"PREPARING_ENVIRONMENT",
	"ANALYZING_REPOSITORY",
	"UNDERSTANDING_ISSUE",
	"INVESTIGATING",
	"ATTEMPTING_REPRODUCTION",
	"REPRODUCED",
	"DIAGNOSING",
	"ROOT_CAUSE_IDENTIFIED",
	"REPRODUCED_AND_DIAGNOSED",
	"REPRODUCED_NOT_DIAGNOSED",
	"CONTRADICTS_SPECIFICATION",
	"ISSUE_ALREADY_RESOLVED",
	"NO_CHANGE_REQUIRED",
	"NO_RUNNABLE_CHECK",
	"REPRODUCTION_FAILED",
	"INSUFFICIENT_EVIDENCE",
	"NEEDS_HUMAN_INPUT",
	"CANCELLED",
	"FAILED",
}

// InvestigationOutcomes is what a finished investigation reports, from OUTCOMES
// in packages/shared/src/states.ts. It is not a query filter; it is here so a
// reader knows what Investigation.Outcome can hold.
var InvestigationOutcomes = []string{
	"REPRODUCED_AND_DIAGNOSED",
	"REPRODUCED_NOT_DIAGNOSED",
	"CONTRADICTS_SPECIFICATION",
	"NO_CHANGE_REQUIRED",
	"NO_RUNNABLE_CHECK",
	"INCONCLUSIVE",
	"CANCELLED",
	"ERRORED",
}

// EvidenceTypes is the type filter vocabulary for InvestigationEvidence, from
// EVIDENCE_TYPES in packages/shared/src/evidence.ts.
var EvidenceTypes = []string{
	"TEST_RESULT",
	"REPRODUCTION",
	"STACK_TRACE",
	"LOG",
	"COMMAND_OUTPUT",
	"HTTP_RESPONSE",
	"BROWSER_OBSERVATION",
	"SCREENSHOT",
	"CODE_REFERENCE",
	"GIT_HISTORY",
	"BUILD_RESULT",
	"TYPECHECK_RESULT",
	"LINT_RESULT",
	"PERFORMANCE_RESULT",
	"VERIFICATION",
	"VALUE_OBSERVATION",
	"SPECIFICATION",
	"VULNERABILITY",
}

// FindingSeverities is the severity filter vocabulary for ValidationFindings,
// from FINDING_SEVERITIES in packages/shared/src/validation.ts.
var FindingSeverities = []string{
	"HIGH",
	"MEDIUM",
	"LOW",
}

// FindingStatuses is the status filter vocabulary for ValidationFindings, from
// FINDING_STATUSES in packages/shared/src/validation.ts.
var FindingStatuses = []string{
	"OPEN",
	"DISMISSED",
	"ENVIRONMENT_RELATED",
	"RESOLVED",
}

// ValidationStates is the state filter vocabulary for ListValidations, from
// VALIDATION_STATES in packages/shared/src/validation.ts.
var ValidationStates = []string{
	"CREATED",
	"ANALYZING_CHANGE",
	"UNDERSTANDING_INTENT",
	"PLANNING",
	"PREPARING_ENVIRONMENT",
	"RUNNING",
	"CONFIRMING_FINDINGS",
	"INVESTIGATING_FINDING",
	"COMPLETED",
	"CANCELLED",
	"FAILED",
}

// ValidationOutcomes is the outcome filter vocabulary for ListValidations, from
// VALIDATION_OUTCOMES in packages/shared/src/validation.ts.
//
// COMPLETED as a state carries no judgement; the outcome is what was concluded.
// BLOCKED means the environment would not come up, so nothing was ever asked of
// the software under test — it is not a product failure and must not be
// rendered as one. ERRORED is Credda breaking, spelled differently from the
// FAILED state on purpose: the state says who stopped, the outcome says what
// was concluded.
var ValidationOutcomes = []string{
	"VERIFIED",
	"FAILED",
	"BLOCKED",
	"INCONCLUSIVE",
	"NO_CHANGE_REQUIRED",
	"CANCELLED",
	"ERRORED",
}

// ResolutionConfidenceClasses is the confidence filter vocabulary for
// ListResolutions, from RESOLUTION_CONFIDENCE_CLASSES in
// packages/shared/src/resolution.ts.
//
// An ordinal class, and ResolutionConfidence has no numeric member at all: no
// score, no ratio, no count a renderer could turn into a percentage. Credda has
// no calibrated probability model, and the field a reviewer reads to decide
// whether to trust a fix is the worst possible place to invent a number.
var ResolutionConfidenceClasses = []string{
	"ESTABLISHED",
	"PARTIALLY_ESTABLISHED",
	"NOT_ESTABLISHED",
}

// LearningKinds is the kind filter vocabulary for RepositoryLearnings, from
// LEARNING_KINDS in packages/memory/src/repository-memory.ts.
var LearningKinds = []string{
	"REPRODUCTION_RECIPE",
	"FRAGILE_SITE",
	"REJECTED_APPROACH",
	"NON_DEFECT",
	"CONVENTION",
}

// EvidenceStrengths is the ordinal scale on Evidence.Strength, from
// EVIDENCE_STRENGTHS. Deliberately ordinal rather than a percentage, for the
// same reason as ResolutionConfidenceClasses.
var EvidenceStrengths = []string{"STRONG", "MODERATE", "WEAK"}

// EvidencePhases says which side of a before/after comparison an artifact
// belongs to, from EVIDENCE_PHASES.
var EvidencePhases = []string{"BEFORE_PATCH", "AFTER_PATCH", "INDEPENDENT"}

// EventSeverities is the severity on an event, from EVENT_SEVERITIES.
//
// Events of severity "debug" are filtered out of the events routes unless
// IncludeDebug is set, and are never sent over a stream at all.
var EventSeverities = []string{"debug", "info", "warn", "error"}

// ── investigations ──────────────────────────────────────────────────────────

// InvestigationSummary is one row of ListInvestigations.
//
// It deliberately carries no RepositoryID: the engine's list serializer omits
// it. Call GetInvestigation for the full record.
type InvestigationSummary struct {
	ID         string  `json:"id"`
	IssueRef   *string `json:"issueRef"`
	IssueTitle string  `json:"issueTitle"`
	State      string  `json:"state"`
	// Outcome is nil until the run reaches a terminal state. It is never a
	// stand-in: an unfinished run has concluded nothing.
	Outcome *string `json:"outcome"`
	// ProviderID names the model provider the run used, or is nil.
	ProviderID  *string `json:"providerId"`
	StartedAt   *string `json:"startedAt"`
	CompletedAt *string `json:"completedAt"`
	CreatedAt   string  `json:"createdAt"`
	// DurationMs is nil unless the run both started and finished. Never negative.
	DurationMs    *int64 `json:"durationMs"`
	EventCount    int    `json:"eventCount"`
	EvidenceCount int    `json:"evidenceCount"`
}

// Investigation is the full record.
type Investigation struct {
	ID           string  `json:"id"`
	OrgID        string  `json:"orgId"`
	RepositoryID string  `json:"repositoryId"`
	IssueRef     *string `json:"issueRef"`
	IssueTitle   string  `json:"issueTitle"`
	IssueBody    string  `json:"issueBody"`
	State        string  `json:"state"`
	Outcome      *string `json:"outcome"`
	ProviderID   *string `json:"providerId"`
	// Budget is the run's budget record as the engine wrote it. Absent on runs
	// created without one. No field of it is an API contract.
	Budget      json.RawMessage `json:"budget,omitempty"`
	StartedAt   *string         `json:"startedAt"`
	CompletedAt *string         `json:"completedAt"`
	// Error is set when the run itself broke, which is Credda failing rather
	// than a defect being found.
	Error      *string `json:"error"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	DurationMs *int64  `json:"durationMs"`
}

// Hypothesis is a candidate cause the run considered, with the evidence for and
// against it. Both lists travel, because a hypothesis shown without what
// contradicts it is an assertion.
type Hypothesis struct {
	ID                       string   `json:"id"`
	InvestigationID          string   `json:"investigationId"`
	Description              string   `json:"description"`
	Rank                     int      `json:"rank"`
	Status                   string   `json:"status"`
	SupportingEvidenceIDs    []string `json:"supportingEvidenceIds"`
	ContradictingEvidenceIDs []string `json:"contradictingEvidenceIds"`
	CreatedAt                string   `json:"createdAt"`
	UpdatedAt                string   `json:"updatedAt"`
}

// Patch is a proposed change: the diff, the files it touches, and why.
//
// The engine's patch path is currently withheld pending a model-backed run, so
// on today's deployments this list is empty. That is a status with a date on it
// and not a statement about what Credda is: writing the fix is the product, and
// the patch path returns on evidence. See ADR 0018.
type Patch struct {
	ID              string   `json:"id"`
	InvestigationID string   `json:"investigationId"`
	Attempt         int      `json:"attempt"`
	UnifiedDiff     string   `json:"unifiedDiff"`
	FilesChanged    []string `json:"filesChanged"`
	Insertions      int      `json:"insertions"`
	Deletions       int      `json:"deletions"`
	Rationale       string   `json:"rationale"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

// Verification is an executed check of a patch: the verdict, and the signals it
// was derived from. The verdict is a mechanical function of the signals, never
// a model's opinion, which is why both travel.
type Verification struct {
	ID              string              `json:"id"`
	InvestigationID string              `json:"investigationId"`
	PatchID         string              `json:"patchId"`
	Verdict         string              `json:"verdict"`
	Signals         VerificationSignals `json:"signals"`
	Notes           *string             `json:"notes"`
	CreatedAt       string              `json:"createdAt"`
}

// VerificationSignals is what was actually run, before and after the patch.
//
// The core claim a fix makes is that the demonstrated failure is gone:
// ReproductionBefore FAIL and ReproductionAfter PASS. RegressionTestBefore /
// After is the test that fails before and passes after — the proof that the
// test agrees with the patch rather than with the author. Every field is one of
// FAIL, PASS, NOT_RUN; Build, Typecheck and Lint may also be NOT_APPLICABLE.
type VerificationSignals struct {
	ReproductionBefore   string `json:"reproductionBefore"`
	ReproductionAfter    string `json:"reproductionAfter"`
	RegressionTestBefore string `json:"regressionTestBefore"`
	RegressionTestAfter  string `json:"regressionTestAfter"`
	// ExistingTests is nil when the repository's own suite was not run. A nil
	// here is not "nothing failed".
	ExistingTests *ExistingTests `json:"existingTests"`
	Build         string         `json:"build"`
	Typecheck     string         `json:"typecheck"`
	Lint          string         `json:"lint"`
}

// ExistingTests is the repository's own suite, as counted by a run of it.
type ExistingTests struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// FailureSignature is a captured observation of a failure: the exact command,
// what it exited with, and what it produced, normalized so two runs of the same
// failure compare equal.
type FailureSignature struct {
	// Command is the exact command that produced this observation.
	Command  string `json:"command"`
	ExitCode *int   `json:"exitCode"`
	// ErrorClass is e.g. "TypeError", "AssertionError", "ENOENT". Nil when not
	// classifiable.
	ErrorClass *string `json:"errorClass"`
	// NormalizedMessage has absolute paths, timestamps and hex addresses
	// stripped, which is what makes the hash stable across machines.
	NormalizedMessage *string `json:"normalizedMessage"`
	// OriginFile is the innermost stack frame inside the repository under
	// investigation, if any.
	OriginFile *string `json:"originFile"`
	OriginLine *int    `json:"originLine"`
	// FailingTestIDs is populated when the command was a test run.
	FailingTestIDs []string `json:"failingTestIds"`
	// ValueObservation is the reported wrong value, when the command was a
	// synthesised value assertion rather than a crashing program. Absent on
	// every crash signature: a silent wrong-value defect has no error class, no
	// stack and no failing test id, and the engine will not fabricate them.
	ValueObservation json.RawMessage `json:"valueObservation,omitempty"`
	// Hash is a stable hash over the fields above, for cheap equality checks.
	Hash string `json:"hash"`
}

// Evidence is one recorded observation. Every material claim Credda makes cites
// evidence IDs, and evidence outlives the sandbox that produced it.
type Evidence struct {
	ID              string `json:"id"`
	InvestigationID string `json:"investigationId"`
	// CheckID is set on evidence reached through a validation, naming the check
	// that cited it. Nil on investigation evidence.
	CheckID *string `json:"checkId,omitempty"`
	Type    string  `json:"type"`
	Phase   string  `json:"phase"`
	// Strength is an ordinal label (see EvidenceStrengths), not a probability.
	Strength string `json:"strength"`
	Summary  string `json:"summary"`
	// ContentRef points into the artifact store at the full captured artifact.
	// This API serves no route that fetches one.
	ContentRef *string         `json:"contentRef"`
	Metadata   json.RawMessage `json:"metadata"`
	// Signature is the captured failure this evidence records, or nil when the
	// observation was not a failure.
	Signature *FailureSignature `json:"signature"`
	CreatedAt string            `json:"createdAt"`
}

// Event is one entry on an investigation's timeline.
type Event struct {
	ID              string `json:"id"`
	InvestigationID string `json:"investigationId"`
	// Sequence is the monotonic cursor. It is what NextSince and a stream's
	// Last-Event-ID carry.
	Sequence int    `json:"sequence"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	// State is the investigation state at the time of the event, when the event
	// was a transition.
	State       *string         `json:"state"`
	AgentRunID  *string         `json:"agentRunId"`
	ToolCallID  *string         `json:"toolCallId"`
	EvidenceIDs []string        `json:"evidenceIds"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   string          `json:"createdAt"`
}

// InvestigationList is the ListInvestigations response.
//
// Total is the count of everything matching the state filter in this
// organisation, not the length of Investigations, so a caller can tell it is
// holding one page.
type InvestigationList struct {
	Investigations []InvestigationSummary `json:"investigations"`
	Total          int                    `json:"total"`
}

// InvestigationDetail is the GetInvestigation and CreateInvestigation response.
//
// Patches and Verifications are empty on a deployment whose patch path is
// withheld. EvidenceCount is a count of the whole evidence set, which is paged
// separately by InvestigationEvidence.
type InvestigationDetail struct {
	Investigation Investigation  `json:"investigation"`
	Hypotheses    []Hypothesis   `json:"hypotheses"`
	Patches       []Patch        `json:"patches"`
	Verifications []Verification `json:"verifications"`
	EvidenceCount int            `json:"evidenceCount"`
	// LatestSequence is the highest event sequence written so far. Compare it
	// with an events page's NextSince to know whether you are caught up.
	LatestSequence int `json:"latestSequence"`
}

// CreationStatus is what CreateInvestigationOnce ACHIEVED: whether THIS call
// opened the run, or whether an earlier request under the same key did.
//
// A named type with constants rather than a bool, for the reason
// CancellationStatus is one. The engine answers the create route with 201 when
// the key was new and 200 when it is handing back a run it already has, and the
// BODY IS IDENTICAL either way — so this is the only thing that says whether a
// model budget was just committed. A shape that dropped it would make every
// retried create look like a fresh run in whatever the caller writes to their
// own ledger.
type CreationStatus string

const (
	// StatusCreated (HTTP 201) means this request opened the run. Nothing under
	// this key had reached the engine before it.
	StatusCreated CreationStatus = "CREATED"
	// StatusReplayed (HTTP 200) means an earlier request under the same key
	// opened the run and this one is being handed that same run back. NOTHING
	// WAS CREATED HERE AND NOTHING WAS BILLED — which is the point: it is what
	// a retried create answers once the first attempt got through.
	StatusReplayed CreationStatus = "REPLAYED"
)

// InvestigationCreation is the CreateInvestigationOnce result.
//
// It is assembled by this client from a response body and its status line, and
// is the one type in this file that is not a transcription of a serializer: the
// engine writes nothing in the body that separates 201 from 200.
//
// The refusal is not a value here. The same key over a DIFFERENT body is a 409
// APIError with CodeIdempotencyKeyReused, disclosing neither run, because it
// neither created nor replayed anything.
type InvestigationCreation struct {
	// Detail is the run, in the same shape GetInvestigation returns.
	Detail *InvestigationDetail
	// Status is what was achieved. Switch on it before recording that a run was
	// opened. Do not treat a nil error as "created".
	Status CreationStatus
	// Key is the key the run is claimed under. Sending it again with the same
	// body returns this same run.
	Key IdempotencyKey
}

// Opened reports whether THIS call opened the run: true for StatusCreated,
// false for StatusReplayed.
//
// A convenience over the switch, not a substitute for reading Status.
func (c InvestigationCreation) Opened() bool { return c.Status == StatusCreated }

// CancellationStatus is what CancelInvestigation ACHIEVED. It is a named type
// with exported constants rather than a bool or a plain string for one reason:
// a caller cannot write `if c.Cancelled`, and cannot compare against a
// misspelled literal that silently never matches. Two of the three values are
// HTTP 200 and one is 202, and they do not mean the same thing about the
// operator's machine or the operator's bill.
type CancellationStatus string

const (
	// StatusCancelled (HTTP 200) means the run had not started. The job was
	// still queued, one conditional UPDATE refused it the claim under the same
	// write lock the claim takes, and the record is now CANCELLED. NOTHING IS
	// RUNNING.
	StatusCancelled CancellationStatus = "CANCELLED"
	// StatusCancellationRequested (HTTP 202) means a worker is INSIDE the run,
	// holding a sandbox and possibly a model call. The request is durable and
	// that worker honours it on the heartbeat it already performs — but the run
	// HAS NOT STOPPED, and the API has written no terminal state. The run
	// writes its own when it lets go. InvestigationEvents and
	// StreamInvestigation are how a caller learns that it did.
	//
	// Rendering this as "cancelled" tells an operator something false about a
	// container that is still cloning and a budget that is still being spent.
	StatusCancellationRequested CancellationStatus = "CANCELLATION_REQUESTED"
	// StatusAlreadyCancelled (HTTP 200) means it was cancelled before this
	// call. Repeating the request is not an error.
	StatusAlreadyCancelled CancellationStatus = "ALREADY_CANCELLED"
)

// Cancellation is the CancelInvestigation response.
//
// The two refusals are NOT values here. A run that already finished is a 409
// APIError with CodeAlreadyFinished, and one executing outside the job queue —
// which is what `credda run` does — is a 409 with CodeNotCancellable. Both are
// errors because neither stopped anything.
type Cancellation struct {
	InvestigationID string `json:"investigationId"`
	// State is the record's state, re-read after the write rather than assumed.
	//
	// It is "CANCELLED" only when Status is StatusCancelled or
	// StatusAlreadyCancelled. On StatusCancellationRequested this is the state
	// the run is STILL IN, because the API does not write CANCELLED there.
	State string `json:"state"`
	// Status is what was achieved. Switch on it. Do not treat a nil error as
	// "stopped".
	Status CancellationStatus `json:"status"`
}

// Stopped reports whether the run is actually stopped: true for
// StatusCancelled and StatusAlreadyCancelled, false for
// StatusCancellationRequested.
//
// A convenience over the switch, not a substitute for reading Status — "why is
// it not stopped" is answered by the constant and not by a false.
func (c Cancellation) Stopped() bool {
	return c.Status == StatusCancelled || c.Status == StatusAlreadyCancelled
}

// EventPage is the InvestigationEvents response.
type EventPage struct {
	Events []Event `json:"events"`
	// LatestSequence is the newest sequence on the investigation, regardless of
	// this page.
	LatestSequence int `json:"latestSequence"`
	// NextSince is the cursor to pass as Since for the following page. It comes
	// from the page BEFORE debug events were filtered out, so resuming from it
	// skips nothing.
	NextSince int `json:"nextSince"`
	// HasMore reports whether the page was truncated. It is computed before the
	// debug filter, so an empty Events with HasMore true means the page held
	// only debug events, not that the timeline ended.
	HasMore bool `json:"hasMore"`
}

// EvidencePage is the InvestigationEvidence response. Total is the size of the
// filtered set, not of the page.
type EvidencePage struct {
	Evidence []Evidence `json:"evidence"`
	Total    int        `json:"total"`
}

// ── repositories ────────────────────────────────────────────────────────────

// Repository is a repository registered with this organisation.
type Repository struct {
	ID    string `json:"id"`
	OrgID string `json:"orgId"`
	Name  string `json:"name"`
	// Source is a clone URL for a remote repository. For a LOCAL CHECKOUT it is
	// not a path: the engine reduces a host path to `local:<final segment>`, so
	// that the operator's own directory layout, username included, does not
	// cross the wire. A value starting `local:` is a label, never something to
	// clone or open.
	Source        string `json:"source"`
	DefaultBranch string `json:"defaultBranch"`
	CreatedAt     string `json:"createdAt"`
}

// RepositoryList is the ListRepositories response.
type RepositoryList struct {
	Repositories []Repository `json:"repositories"`
	Total        int          `json:"total"`
}

// Learning is something Credda has learned about a repository across runs.
//
// Weight is an ordinal label derived from Observations, not a probability.
// The engine's `metadata` bag is deliberately not served on this record: it is
// written by the engine and has no reviewed client contract.
type Learning struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	// FilePath and Symbol are set on learnings anchored to a place in the code,
	// such as a FRAGILE_SITE, and nil on the rest.
	FilePath *string `json:"filePath"`
	Symbol   *string `json:"symbol"`
	// Observations is how many times this has been independently observed.
	Observations int    `json:"observations"`
	Weight       string `json:"weight"`
	// InvestigationIDs are the runs that produced or reinforced this.
	InvestigationIDs []string `json:"investigationIds"`
	CreatedAt        string   `json:"createdAt"`
	LastSeenAt       string   `json:"lastSeenAt"`
}

// LearningPage is the RepositoryLearnings response.
//
// An empty page is a real answer: a repository Credda has learned nothing about
// yet is an empty list, not a 404. "We know nothing here" is information.
type LearningPage struct {
	Learnings []Learning `json:"learnings"`
	Total     int        `json:"total"`
}

// ── resolutions ─────────────────────────────────────────────────────────────

// ResolutionSummary is one row of ListResolutions.
//
// ConfidenceClass and NotEstablished travel together, always. They are one fact
// written twice so that neither can be read alone: a class with its gaps a
// request away would be exactly the bare assertion the pairing exists to
// prevent. Do not render one without the other.
type ResolutionSummary struct {
	ID              string `json:"id"`
	InvestigationID string `json:"investigationId"`
	// Reported is the report's own title, quoted. Never Credda's description of
	// the defect.
	Reported  string  `json:"reported"`
	Reference *string `json:"reference"`
	SignalID  *string `json:"signalId"`
	// ReproductionStatus says whether the reported failure was actually
	// observed to happen.
	ReproductionStatus string `json:"reproductionStatus"`
	// VerificationVerdict is nil when no verification run exists. Never a
	// stand-in verdict: nil means nothing verified this, which is different
	// from something verifying it and failing.
	VerificationVerdict *string `json:"verificationVerdict"`
	RegressionStatus    string  `json:"regressionStatus"`
	ConfidenceClass     string  `json:"confidenceClass"`
	// NotEstablished is what this record does NOT establish, in a reviewer's
	// terms. Empty exactly when ConfidenceClass is ESTABLISHED.
	NotEstablished []string `json:"notEstablished"`
	CreatedAt      string   `json:"createdAt"`
}

// Resolution is the whole record of what one run established.
//
// RootCause, Fix and Verification are nil exactly when the run produced no such
// row, and the hole is named in Confidence.NotEstablished rather than filled
// in. Read the gaps.
type Resolution struct {
	ID                   string               `json:"id"`
	InvestigationID      string               `json:"investigationId"`
	Bug                  ResolutionBug        `json:"bug"`
	Evidence             []ResolutionCitation `json:"evidence"`
	Reproduction         ResolutionRepro      `json:"reproduction"`
	RootCause            *ResolutionCause     `json:"rootCause"`
	Fix                  *ResolutionFix       `json:"fix"`
	Verification         *ResolutionVerify    `json:"verification"`
	RegressionProtection ResolutionRegression `json:"regressionProtection"`
	Confidence           ResolutionConfidence `json:"confidence"`
	CreatedAt            string               `json:"createdAt"`
}

// ResolutionBug is what was reported, and where.
type ResolutionBug struct {
	Reported  string  `json:"reported"`
	Reference *string `json:"reference"`
	SignalID  *string `json:"signalId"`
	// AffectedFiles are the files the supporting evidence's own signatures
	// named — not a guess about where the bug lives.
	AffectedFiles []string `json:"affectedFiles"`
}

// ResolutionCitation is an evidence pointer on a resolution: enough to identify
// the record, not the record itself.
type ResolutionCitation struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

// ResolutionRepro is whether the reported failure was made to happen, and what
// made it happen.
type ResolutionRepro struct {
	Status string `json:"status"`
	// Command is read off the signature, so it is never a command that was not
	// the one that produced the capture.
	Command    *string           `json:"command"`
	Signature  *FailureSignature `json:"signature"`
	EvidenceID *string           `json:"evidenceId"`
	// TimedOutAttempts is nil, not 0, on a record written before the column
	// existed. Nil means "this record does not say"; 0 means "nothing was
	// killed". They are different answers.
	TimedOutAttempts *int `json:"timedOutAttempts"`
}

// ResolutionCause is the cause the run established, and the evidence for it.
type ResolutionCause struct {
	HypothesisID          string   `json:"hypothesisId"`
	Description           string   `json:"description"`
	SupportingEvidenceIDs []string `json:"supportingEvidenceIds"`
	AnchoredFiles         []string `json:"anchoredFiles"`
}

// ResolutionFix is the patch the run wrote. Nil on the parent when no patch was
// produced.
type ResolutionFix struct {
	PatchID      string   `json:"patchId"`
	Attempt      int      `json:"attempt"`
	FilesChanged []string `json:"filesChanged"`
	Insertions   int      `json:"insertions"`
	Deletions    int      `json:"deletions"`
	Rationale    string   `json:"rationale"`
}

// ResolutionVerify is the executed proof of the fix.
type ResolutionVerify struct {
	VerificationRunID string              `json:"verificationRunId"`
	Verdict           string              `json:"verdict"`
	Signals           VerificationSignals `json:"signals"`
	EvidenceIDs       []string            `json:"evidenceIds"`
}

// ResolutionRegression is the test that proves the fix: Before and After are
// the same test run on either side of the patch. FAIL then PASS is the pair a
// reviewer is looking for, and it is what tells a fixed failure from a mutated
// one.
type ResolutionRegression struct {
	Status string `json:"status"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// ResolutionConfidence is the class and the gaps, together. There is no numeric
// member here by design: see ResolutionConfidenceClasses.
type ResolutionConfidence struct {
	Class string `json:"class"`
	// NotEstablished is empty exactly when Class is ESTABLISHED.
	NotEstablished []string `json:"notEstablished"`
}

// ResolutionList is the ListResolutions response.
type ResolutionList struct {
	Resolutions []ResolutionSummary `json:"resolutions"`
	Total       int                 `json:"total"`
}

// ── validations ─────────────────────────────────────────────────────────────

// ValidationSummary is one row of ListValidations, the review queue.
//
// EnvironmentStatus is on the row deliberately: a run that ended BLOCKED must
// read as blocked in the queue, not as a failure of the change.
type ValidationSummary struct {
	ID           string `json:"id"`
	RepositoryID string `json:"repositoryId"`
	// RepositorySource is nil only when the repository row is gone; never a
	// stand-in label. Reduced to `local:<name>` for a local checkout, as on
	// Repository.Source.
	RepositorySource *string `json:"repositorySource"`
	SourceType       string  `json:"sourceType"`
	SourceRef        string  `json:"sourceRef"`
	BaseCommit       *string `json:"baseCommit"`
	HeadCommit       *string `json:"headCommit"`
	State            string  `json:"state"`
	Outcome          *string `json:"outcome"`
	TriggerKind      string  `json:"triggerKind"`
	// EnvironmentStatus and EnvironmentFailureKind say whether the environment
	// came up. An environment failure is not a product failure.
	EnvironmentStatus      string  `json:"environmentStatus"`
	EnvironmentFailureKind *string `json:"environmentFailureKind"`
	// ExecutableFilesChanged is nil when the change was never analysed. Zero
	// means it was analysed and touched nothing executable, which is the
	// NO_CHANGE_REQUIRED outcome and a success.
	ExecutableFilesChanged *int    `json:"executableFilesChanged"`
	StartedAt              *string `json:"startedAt"`
	CompletedAt            *string `json:"completedAt"`
	CreatedAt              string  `json:"createdAt"`
	DurationMs             *int64  `json:"durationMs"`
}

// Validation is the full record of one validation run.
type Validation struct {
	ID           string  `json:"id"`
	OrgID        string  `json:"orgId"`
	RepositoryID string  `json:"repositoryId"`
	SourceType   string  `json:"sourceType"`
	SourceRef    string  `json:"sourceRef"`
	BaseCommit   *string `json:"baseCommit"`
	HeadCommit   *string `json:"headCommit"`
	State        string  `json:"state"`
	Outcome      *string `json:"outcome"`
	TriggerKind  string  `json:"triggerKind"`
	TriggeredBy  *string `json:"triggeredBy"`
	// IntentSummary is what the engine understood the change to be trying to
	// do. Nil before the UNDERSTANDING_INTENT phase runs.
	IntentSummary          *string `json:"intentSummary"`
	ExecutableFilesChanged *int    `json:"executableFilesChanged"`
	StartedAt              *string `json:"startedAt"`
	CompletedAt            *string `json:"completedAt"`
	Error                  *string `json:"error"`
	CreatedAt              string  `json:"createdAt"`
	UpdatedAt              string  `json:"updatedAt"`
	DurationMs             *int64  `json:"durationMs"`
}

// Environment is whether the run's environment came up, and what was attempted.
//
// Detail is what was tried — runtime, package manager, commands, the stderr of
// the install that failed — passed through as the engine recorded it rather
// than summarised, because a BLOCKED report whose reason has been summarised
// away is the report nobody can act on. Its keys are the engine's, and no field
// of it is an API contract.
type Environment struct {
	Status      string          `json:"status"`
	FailureKind *string         `json:"failureKind"`
	Detail      json.RawMessage `json:"detail"`
}

// ValidationDetail is the GetValidation response.
//
// CheckCount is on the detail rather than a page away for a reason: a completed
// run with zero checks is the false success this product exists to prevent, and
// a caller must be able to see it without a second request.
type ValidationDetail struct {
	Validation  Validation  `json:"validation"`
	Environment Environment `json:"environment"`
	// ChangeImpact is the engine's analysis of what the change touches, as it
	// recorded it. No field of it is an API contract.
	ChangeImpact   json.RawMessage `json:"changeImpact"`
	CheckCount     int             `json:"checkCount"`
	FindingCount   int             `json:"findingCount"`
	EvidenceCount  int             `json:"evidenceCount"`
	LatestSequence int             `json:"latestSequence"`
}

// ValidationCheck is one entry of the plan: what was checked, why, and how it
// went.
//
// BaseStatus is the whole product on one field. A check that FAILED is re-run
// against the base commit before it may become a finding; BaseStatus PASSED
// there means this change caused the failure, and that is the difference
// between "this change broke it" and "it was already broken". It is never
// omitted.
type ValidationCheck struct {
	ID           string `json:"id"`
	ValidationID string `json:"validationId"`
	Sequence     int    `json:"sequence"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Method       string `json:"method"`
	// Reason is why this check exists, so the plan can be read rather than
	// trusted.
	Reason           string  `json:"reason"`
	Target           *string `json:"target"`
	ExpectedBehavior *string `json:"expectedBehavior"`
	// RequirementSource is where the expected behaviour was read from.
	RequirementSource *string `json:"requirementSource"`
	Status            string  `json:"status"`
	BaseStatus        *string `json:"baseStatus"`
	// InvestigationID links a check that was escalated into a full
	// investigation.
	InvestigationID *string         `json:"investigationId"`
	StartedAt       *string         `json:"startedAt"`
	CompletedAt     *string         `json:"completedAt"`
	Detail          json.RawMessage `json:"detail"`
	CreatedAt       string          `json:"createdAt"`
	DurationMs      *int64          `json:"durationMs"`
}

// Finding is something the validation concluded was wrong.
//
// There is no Evidence list here, deliberately: the findings table holds no
// evidence references, and the evidence a finding rests on is reached through
// its CheckID. An empty list here would put nothing where a reader expects
// proof.
type Finding struct {
	ID           string `json:"id"`
	ValidationID string `json:"validationId"`
	// CheckID is the check that produced this. It is the route to the evidence.
	CheckID  *string `json:"checkId"`
	Title    string  `json:"title"`
	Severity string  `json:"severity"`
	// Confidence is an ordinal label, not a percentage.
	Confidence       string  `json:"confidence"`
	Status           string  `json:"status"`
	ExpectedBehavior *string `json:"expectedBehavior"`
	ObservedBehavior *string `json:"observedBehavior"`
	// Reproduction is how to make it happen again.
	Reproduction *string `json:"reproduction"`
	AffectedArea *string `json:"affectedArea"`
	// LikelySource is where the engine believes this comes from. "Likely" is
	// the word the engine uses; it is not an established cause.
	LikelySource *string `json:"likelySource"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

// ValidationEvent is one entry on a validation's timeline. Separate from Event
// because validation_events and investigation_events are different tables with
// different columns.
type ValidationEvent struct {
	ID           string  `json:"id"`
	ValidationID string  `json:"validationId"`
	CheckID      *string `json:"checkId"`
	Sequence     int     `json:"sequence"`
	Type         string  `json:"type"`
	Severity     string  `json:"severity"`
	Summary      string  `json:"summary"`
	State        *string `json:"state"`
	// Data is the engine's payload for this event. No field of it is an API
	// contract.
	Data      json.RawMessage `json:"data"`
	CreatedAt string          `json:"createdAt"`
}

// ValidationList is the ListValidations response.
type ValidationList struct {
	Validations []ValidationSummary `json:"validations"`
	Total       int                 `json:"total"`
}

// CheckPage is the ValidationChecks response. Total is the size of the whole
// plan, so a caller can tell it holds only part of it.
type CheckPage struct {
	Checks []ValidationCheck `json:"checks"`
	Total  int               `json:"total"`
}

// FindingPage is the ValidationFindings response.
type FindingPage struct {
	Findings []Finding `json:"findings"`
	Total    int       `json:"total"`
}

// ValidationEventPage is the ValidationEvents response. Same cursor semantics
// as EventPage.
type ValidationEventPage struct {
	Events         []ValidationEvent `json:"events"`
	LatestSequence int               `json:"latestSequence"`
	NextSince      int               `json:"nextSince"`
	HasMore        bool              `json:"hasMore"`
}

// ── organization ────────────────────────────────────────────────────────────

// Organization is the workspace an API key speaks for.
type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"createdAt"`
}

// OrganizationDetail is the GetOrganization response: the organisation and what
// it holds.
//
// Every count is a count over one scoped table. Investigations and validations
// are counted separately because they are different runs, and a sum of them is
// a number nobody can take apart again. There is no plan, seat allowance or
// spend record here because the engine's schema holds none.
type OrganizationDetail struct {
	Organization Organization `json:"organization"`
	MemberCount  int          `json:"memberCount"`
	// APIKeyCount counts only keys that can actually authenticate. Revoked ones
	// are counted apart, because a revoked key is refused at the door and
	// including it would claim more credentials reach this organisation than
	// the server will accept.
	APIKeyCount        int `json:"apiKeyCount"`
	RevokedAPIKeyCount int `json:"revokedApiKeyCount"`
	RepositoryCount    int `json:"repositoryCount"`
	InvestigationCount int `json:"investigationCount"`
	ValidationCount    int `json:"validationCount"`
}

// Avatar is derived from the member row, never fetched. The API picks a colour
// BUCKET, not a colour: a client with N swatches takes ColorIndex % N. Kind is
// always "INITIALS" today.
type Avatar struct {
	Kind string `json:"kind"`
	// Initials is one or two characters, never empty.
	Initials string `json:"initials"`
	// ColorIndex is stable per user, in 0..7.
	ColorIndex int `json:"colorIndex"`
}

// OrganizationMember is one membership row.
//
// Role and RoleEnforced travel as a pair, and RoleEnforced is false. The reason
// is structural: api_keys has an org_id and no user_id, so an authenticated
// request identifies an organisation and never a person, and there is no member
// on the request for a role to be looked up for. Serving the label is still
// worth doing — "the operator wrote OWNER next to this person" is true — but a
// Role column rendered without RoleEnforced is a rendered access model, and
// this product has none. Do not render one without the other.
type OrganizationMember struct {
	UserID string  `json:"userId"`
	Email  string  `json:"email"`
	Name   *string `json:"name"`
	Avatar Avatar  `json:"avatar"`
	Role   string  `json:"role"`
	// RoleEnforced is false. Read the doc comment on this type.
	RoleEnforced bool   `json:"roleEnforced"`
	JoinedAt     string `json:"joinedAt"`
}

// MemberPage is the OrganizationMembers response.
//
// Total is the number of membership ROWS, which is not the same claim as the
// number of people with access: nothing in the engine writes users or
// organization_members, so on an install where those rows were never created
// this is an empty list next to a working API key. Render that as "no member
// records exist", never as "you are alone in here".
type MemberPage struct {
	Members []OrganizationMember `json:"members"`
	Total   int                  `json:"total"`
}

// APIKey is a key that can reach this organisation. Revoked keys are included,
// because "this key was revoked on the 3rd" is the answer an operator came for.
//
// Nothing here is secret and nothing is omitted: api_keys stores a SHA-256 of
// the secret half and nothing else, so there is no field from which a token
// could be rebuilt. ID is the public half by design.
type APIKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	// LastUsedAt is COARSE by design: written at most once a minute per key. It
	// answers "is this key still in use", not "when was the last request". Nil
	// when the key has never been used.
	LastUsedAt *string `json:"lastUsedAt"`
	// RevokedAt non-nil means the key is refused. Verification stops at this
	// field, and an open event stream is cut within a second of it being set.
	RevokedAt *string `json:"revokedAt"`
}

// APIKeyPage is the OrganizationKeys response.
type APIKeyPage struct {
	Keys  []APIKey `json:"keys"`
	Total int      `json:"total"`
}

// ── health ──────────────────────────────────────────────────────────────────

// ReadinessCheck is one thing that was checked, and how.
//
// Status is "ok", "failed", or "unknown". "unknown" is NOT a softer "failed":
// it means the check could not be run at all, which is still not evidence of
// readiness. A probe that reports healthy because it had nothing to look at is
// the same class of claim as a verdict with no evidence.
type ReadinessCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Readiness is the GetHealth response.
//
// Every claim in it is established by performing the thing it claims: the
// database is queried, the schema version is read and compared, and the
// artifact store is checked by writing and removing a probe file. Status is
// "ok" or "degraded"; a degraded readiness arrives as an APIError with status
// 503, so a caller that gets a Readiness back without an error is ready.
type Readiness struct {
	Status string `json:"status"`
	// SchemaVersion is nil when the version could not be read at all.
	SchemaVersion *int `json:"schemaVersion"`
	// ExpectedSchemaVersion is the version this engine BUILD's migrations
	// produce. A mismatch with SchemaVersion is the migrations check failing.
	ExpectedSchemaVersion int              `json:"expectedSchemaVersion"`
	Checks                []ReadinessCheck `json:"checks"`
}
