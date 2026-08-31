package credda

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Every method in this file is one route mounted in apps/api/src/app.ts. The
// path in each doc comment is the literal path the engine serves.
//
// All of them are GET except CreateInvestigation, CreateInvestigationOnce (the
// same route, under an idempotency key) and CancelInvestigation. That is not an
// omission: the API mounts exactly two POST routes and no PATCH or DELETE at
// all. Resolution records are never revised, so the repository has no
// update method and the route module has no writing route to add one; keys are
// minted out of band by the operator, because nothing in the engine separates
// an OWNER from a VIEWER and a key that could mint keys would make every key a
// key factory.
//
// Every query parameter and body field these methods send is one the engine's
// zod schemas declare. That is load-bearing rather than tidy: those schemas are
// .strict(), so a key they do not define is a 400 CodeValidationFailed naming
// it, where an unknown one was once accepted and ignored.

// ── investigations ──────────────────────────────────────────────────────────

// InvestigationQuery filters ListInvestigations.
type InvestigationQuery struct {
	// State filters to one investigation state. Must be a member of
	// InvestigationStates; anything else is a 400 VALIDATION_FAILED.
	State string
	// Repository filters to one repository. An unknown one is a 404 rather
	// than an empty page, for the reason on ResolutionQuery.
	Repository string
	// Signal filters to the investigations one signal caused. An unknown one
	// is a 404, for the same reason.
	Signal string
	// Outcome filters to one outcome. Must be a member of
	// InvestigationOutcomes.
	Outcome string
	Page
}

// Two filters the engine's listQuery declares and this struct does not carry:
// `hasSignal`, which asks WHETHER a run was raised by a reported failure rather
// than which one raised it, and `issueRef`, the provenance filter that answers
// "did I already file this?" — and which is the one filter on that route that
// does not 404 on a value nothing matches. Neither is reachable from here.

// ListInvestigations returns a page of investigations, newest first.
//
// GET /api/investigations
//
// Total is counted under the same filters the page was cut with — every one of
// them, not State alone — so it does not shrink as you page.
func (c *Client) ListInvestigations(ctx context.Context, q *InvestigationQuery) (*InvestigationList, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "state", q.State)
		setStr(qs, "repository", q.Repository)
		setStr(qs, "signal", q.Signal)
		setStr(qs, "outcome", q.Outcome)
		q.Page.apply(qs)
	}
	var out InvestigationList
	if err := c.get(ctx, withQuery("/investigations", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInvestigationInput is the body of CreateInvestigation.
//
// It is a strict subset of the engine's createBody: `start` and `budget` are
// declared there and are not here, so no request this client builds can queue a
// run or lower its ceiling. The schema is .strict(), so nothing may be added to
// this struct that the engine does not declare — but the reverse gap, a field
// the engine declares and this struct omits, fails nothing and is what happened.
type CreateInvestigationInput struct {
	// RepositoryID must name a repository in this organisation. One that does
	// not, or that belongs to another organisation, is a 404.
	RepositoryID string `json:"repositoryId"`
	// IssueTitle is required, 1 to 500 characters.
	IssueTitle string `json:"issueTitle"`
	// IssueBody is the report, up to 100,000 characters. It may be empty.
	IssueBody string `json:"issueBody"`
	// IssueRef is the reporter's own identifier for this — a ticket URL, an
	// issue number. Optional, up to 500 characters.
	IssueRef *string `json:"issueRef,omitempty"`
}

// CreateInvestigation records an investigation and returns it in state
// CREATED.
//
// POST /api/investigations
//
// AS THIS CLIENT SENDS IT, IT RUNS NOTHING. The engine's create route takes a
// `start` boolean that commits the row and its job in one write, and an
// optional downward-only `budget` beside it; CreateInvestigationInput carries
// neither, so the body always omits them, `start` defaults to false at the
// engine, and this route writes a row nothing will claim until a webhook or a
// `credda run` picks the work up. The returned detail therefore has no events,
// no evidence and no outcome, and Start reads "NOT_REQUESTED".
//
// Read InvestigationDetail.Start rather than assuming that. It is the route's
// own statement about what it did, and it is the field to check if this package
// later learns to ask for a run. Watch Stream or poll InvestigationEvents for
// what happens next.
//
// This call sends no Idempotency-Key and is never retried, even with
// WithRetries. Without that header the route behaves exactly as it did before
// the header existed — one run per request — so a repeat opens, and bills for, a
// second investigation into the same report. That is the right default for a
// caller who has not said two requests are one intent, and it is not the call to
// make from a job queue that will retry you: use CreateInvestigationOnce.
//
// The whole request body must be under 256KB or the engine refuses it unread
// with a 413.
func (c *Client) CreateInvestigation(ctx context.Context, in CreateInvestigationInput) (*InvestigationDetail, error) {
	var out InvestigationDetail
	if err := c.post(ctx, "/investigations", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInvestigationOnce enqueues an investigation under an idempotency key —
// the one write on this client that WithRetries will repeat.
//
// POST /api/investigations, with an Idempotency-Key header.
//
// Running an investigation spends a model budget, so a create sent twice because
// a socket died is a second bill. Under a key the engine returns the run the
// first request created, with 200 instead of 201, so repeating this call is
// exactly-once at the server and retrying it is safe.
//
//	req, err := credda.NewIdempotentCreate(credda.CreateInvestigationInput{
//		RepositoryID: repoID, IssueTitle: title, IssueBody: report,
//	})
//	if err != nil {
//		return err
//	}
//	if err := jobs.Record(ticketID, req.Key()); err != nil {
//		return err
//	}
//	created, err := client.CreateInvestigationOnce(ctx, req)
//	if err != nil {
//		return err
//	}
//	if created.Opened() {
//		// A run just opened, and a budget with it.
//	}
//	// Otherwise StatusReplayed: an earlier attempt of ours got through. Nothing
//	// was created here and nothing was billed.
//
// A NIL ERROR DOES NOT MEAN A RUN WAS OPENED. Both 201 and 200 are successes,
// and only Status separates them.
//
// The key and the body arrive together, in one IdempotentCreate, because the
// engine's other answer is a refusal: the same key over a DIFFERENT body is a
// 409 *APIError with CodeIdempotencyKeyReused, disclosing neither run. Making
// the pair as a unit is what keeps a key from drifting onto a report it was not
// minted for; there is no overload here taking the two separately, and the zero
// IdempotentCreate is refused rather than sent without a header.
//
// The claim is scoped to the organisation the API key names and never expires.
// It is deleted when the investigation is.
func (c *Client) CreateInvestigationOnce(ctx context.Context, req IdempotentCreate) (*InvestigationCreation, error) {
	if req.key == "" {
		return nil, fmt.Errorf("%w: build the request with NewIdempotentCreate", ErrInvalidIdempotencyKey)
	}
	var out InvestigationDetail
	status, err := c.postOnce(ctx, "/investigations", req.key, req.input, &out)
	if err != nil {
		return nil, err
	}
	// 201 is the run this request opened; every other success on this route is
	// the engine handing back one it already had. Read off the status line,
	// because the two bodies are identical.
	result := &InvestigationCreation{Detail: &out, Status: StatusReplayed, Key: req.key}
	if status == http.StatusCreated {
		result.Status = StatusCreated
	}
	return result, nil
}

// CancelInvestigationInput is the body of CancelInvestigation.
type CancelInvestigationInput struct {
	// Reason is recorded against the run. Optional, 1 to 500 characters: the
	// engine's cancelBody makes it optional because a cancel with nothing said
	// is still a cancel. An empty string is not sent.
	Reason string `json:"reason,omitempty"`
}

// CancelInvestigation stops a run — or records that one has been ASKED to stop,
// and tells you which of those happened.
//
// POST /api/investigations/{id}/cancel
//
// A cancel that reported success over a container still cloning a repository,
// still running a test suite and still spending a model budget would have told
// an operator something false about their own machine and their own bill. So
// the route reports what it ACHIEVED, and this method hands that back whole
// rather than reducing it to an error-or-nil:
//
//	c, err := client.CancelInvestigation(ctx, id, credda.CancelInvestigationInput{
//		Reason: "wrong repository",
//	})
//	if err != nil {
//		return err
//	}
//	switch c.Status {
//	case credda.StatusCancellationRequested:
//		// A worker is still inside the run. It stops on its next heartbeat and
//		// writes its own terminal state; watch StreamInvestigation for it.
//	default:
//		// c.State is "CANCELLED". Nothing is running.
//	}
//
// A NIL ERROR DOES NOT MEAN THE RUN STOPPED. Both 200 and 202 are successes at
// the transport level, and only Status separates them. Cancellation.Stopped is
// the short form of the switch above.
//
// Two refusals come back as a 409 *APIError, because neither stopped anything:
// CodeAlreadyFinished when the run reached a terminal state, and
// CodeNotCancellable when it is executing outside the job queue.
//
// Repeating the call is safe — an already-cancelled run answers
// StatusAlreadyCancelled rather than an error. It is still never retried under
// WithRetries, like every non-GET: a repeat that crosses a worker's heartbeat
// answers a different status than the attempt it replaced, and a retry policy
// that swallowed that would hand back "cancelled" for a call that was told
// "requested".
func (c *Client) CancelInvestigation(ctx context.Context, id string, in CancelInvestigationInput) (*Cancellation, error) {
	var out Cancellation
	if err := c.post(ctx, "/investigations/"+esc(id)+"/cancel", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetInvestigation returns one investigation with its hypotheses, patches and
// verification runs.
//
// GET /api/investigations/{id}
func (c *Client) GetInvestigation(ctx context.Context, id string) (*InvestigationDetail, error) {
	var out InvestigationDetail
	if err := c.get(ctx, "/investigations/"+esc(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EventQuery pages an events route.
type EventQuery struct {
	// Since is an exclusive cursor: pass the previous page's NextSince. 0 (the
	// default) starts from the beginning.
	Since *int
	// Limit caps the page at 1000 for investigations and 500 for validations,
	// which are also the defaults.
	Limit *int
	// IncludeDebug keeps severity-"debug" events in the page. They are dropped
	// by default, and are never sent over a stream at all.
	IncludeDebug *bool
}

func (q *EventQuery) values() url.Values {
	qs := url.Values{}
	if q == nil {
		return qs
	}
	setInt(qs, "since", q.Since)
	setInt(qs, "limit", q.Limit)
	setBool(qs, "includeDebug", q.IncludeDebug)
	return qs
}

// InvestigationEvents returns a page of the investigation's timeline.
//
// GET /api/investigations/{id}/events
//
// The debug filter runs AFTER the page is cut, so an empty Events with HasMore
// true means the page held only debug events rather than that the timeline
// ended. Resume from NextSince, which comes from the page and not from the
// surviving events.
func (c *Client) InvestigationEvents(ctx context.Context, id string, q *EventQuery) (*EventPage, error) {
	var out EventPage
	if err := c.get(ctx, withQuery("/investigations/"+esc(id)+"/events", q.values()), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EvidenceQuery filters InvestigationEvidence.
type EvidenceQuery struct {
	// Type filters to one evidence type. Must be a member of EvidenceTypes.
	Type string
	Page
}

// InvestigationEvidence returns a page of the evidence an investigation
// recorded.
//
// GET /api/investigations/{id}/evidence
//
// Total is the size of the filtered set, not of the page.
func (c *Client) InvestigationEvidence(ctx context.Context, id string, q *EvidenceQuery) (*EvidencePage, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "type", q.Type)
		q.Page.apply(qs)
	}
	var out EvidencePage
	if err := c.get(ctx, withQuery("/investigations/"+esc(id)+"/evidence", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── repositories ────────────────────────────────────────────────────────────

// ListRepositories returns the repositories registered with this organisation.
//
// GET /api/repositories
func (c *Client) ListRepositories(ctx context.Context, p *Page) (*RepositoryList, error) {
	qs := url.Values{}
	p.apply(qs)
	var out RepositoryList
	if err := c.get(ctx, withQuery("/repositories", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRepository returns one repository by id.
//
// GET /api/repositories/{id}
//
// Every investigation and validation carries a RepositoryID; this is what
// resolves one without paging ListRepositories until the id turns up. An
// unknown id, or one in another organisation, is a 404.
//
// The response also carries `investigations` and `validations`, the two counts
// over this repository under the caller's scope, and this method discards them:
// it returns the record alone. They are counted apart at the engine because
// they are different runs and a sum of them cannot be taken apart again.
func (c *Client) GetRepository(ctx context.Context, id string) (*Repository, error) {
	var out struct {
		Repository Repository `json:"repository"`
	}
	if err := c.get(ctx, "/repositories/"+esc(id), &out); err != nil {
		return nil, err
	}
	return &out.Repository, nil
}

// LearningQuery filters RepositoryLearnings.
type LearningQuery struct {
	// Kind filters to one learning kind. Must be a member of LearningKinds.
	Kind string
	Page
}

// RepositoryLearnings returns what Credda has learned about a repository across
// runs: reproduction recipes that worked here, sites that have been confirmed
// causes, approaches a human rejected, reports that needed no change, and
// conventions worth respecting.
//
// GET /api/repositories/{id}/learnings
//
// A repository with nothing learned yet is an empty list, not a 404.
func (c *Client) RepositoryLearnings(ctx context.Context, id string, q *LearningQuery) (*LearningPage, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "kind", q.Kind)
		q.Page.apply(qs)
	}
	var out LearningPage
	if err := c.get(ctx, withQuery("/repositories/"+esc(id)+"/learnings", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── resolutions ─────────────────────────────────────────────────────────────

// ResolutionQuery filters ListResolutions.
//
// An unknown Investigation or Signal is a 404, not an empty page: "this
// investigation resolved nothing" and "there is no such investigation" are
// different answers, and a caller handed an empty list cannot tell them apart.
type ResolutionQuery struct {
	// Investigation filters to one investigation's records.
	Investigation string
	// Signal filters to one signal's records.
	Signal string
	// Confidence filters by class. Must be a member of
	// ResolutionConfidenceClasses.
	//
	// Confidence=NOT_ESTABLISHED across every investigation is the query that
	// keeps this product honest with itself: it is every record nothing
	// verified.
	Confidence string
	Page
}

// The engine's listQuery also declares `hasSignal` here, and this struct does
// not carry it.

// ListResolutions returns a page of resolution records.
//
// GET /api/resolutions
func (c *Client) ListResolutions(ctx context.Context, q *ResolutionQuery) (*ResolutionList, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "investigation", q.Investigation)
		setStr(qs, "signal", q.Signal)
		setStr(qs, "confidence", q.Confidence)
		q.Page.apply(qs)
	}
	var out ResolutionList
	if err := c.get(ctx, withQuery("/resolutions", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LatestResolution returns the most recent resolution record for one
// investigation, or nil.
//
// GET /api/resolutions/latest?investigation={id}
//
// A nil resolution with a nil error means the investigation EXISTS and has
// produced no record yet, which is a real answer and the one an investigation
// page renders. An unknown investigation id is a 404, so the two are never
// confused. The investigation id is required: "the latest record" with no
// investigation named is not a question.
func (c *Client) LatestResolution(ctx context.Context, investigationID string) (*Resolution, error) {
	qs := url.Values{}
	qs.Set("investigation", investigationID)
	var out struct {
		Resolution *Resolution `json:"resolution"`
	}
	if err := c.get(ctx, withQuery("/resolutions/latest", qs), &out); err != nil {
		return nil, err
	}
	return out.Resolution, nil
}

// GetResolution returns one resolution record in full.
//
// GET /api/resolutions/{id}
func (c *Client) GetResolution(ctx context.Context, id string) (*Resolution, error) {
	var out struct {
		Resolution Resolution `json:"resolution"`
	}
	if err := c.get(ctx, "/resolutions/"+esc(id), &out); err != nil {
		return nil, err
	}
	return &out.Resolution, nil
}

// ── validations ─────────────────────────────────────────────────────────────

// ValidationQuery filters ListValidations.
type ValidationQuery struct {
	// Repository filters to one repository. An unknown one is a 404 rather than
	// an empty page, for the reason on ResolutionQuery.
	Repository string
	// State must be a member of ValidationStates.
	State string
	// Outcome must be a member of ValidationOutcomes.
	Outcome string
	Page
}

// The engine's listQuery also declares `sourceRef`, and this struct does not
// carry it.

// ListValidations returns a page of validation runs — the review queue.
//
// GET /api/validations
func (c *Client) ListValidations(ctx context.Context, q *ValidationQuery) (*ValidationList, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "repository", q.Repository)
		setStr(qs, "state", q.State)
		setStr(qs, "outcome", q.Outcome)
		q.Page.apply(qs)
	}
	var out ValidationList
	if err := c.get(ctx, withQuery("/validations", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetValidation returns one validation with its environment, change impact and
// counts.
//
// GET /api/validations/{id}
func (c *Client) GetValidation(ctx context.Context, id string) (*ValidationDetail, error) {
	var out ValidationDetail
	if err := c.get(ctx, "/validations/"+esc(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidationChecks returns a page of the run's plan, in sequence order.
//
// GET /api/validations/{id}/checks
//
// Total is the size of the whole plan. Read BaseStatus on every failing check:
// it is what separates a failure this change caused from one that was already
// there.
//
// The engine's checksQuery declares a `status` filter over CHECK_STATUSES and
// counts Total under it; this method takes only a page, so "which of them
// failed" still means pulling the plan and tallying it here.
func (c *Client) ValidationChecks(ctx context.Context, id string, p *Page) (*CheckPage, error) {
	qs := url.Values{}
	p.apply(qs)
	var out CheckPage
	if err := c.get(ctx, withQuery("/validations/"+esc(id)+"/checks", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindingQuery filters ValidationFindings. Severity and Status narrow with AND;
// each is one token, not a set.
type FindingQuery struct {
	// Severity must be a member of FindingSeverities.
	Severity string
	// Status must be a member of FindingStatuses.
	Status string
	Page
}

// ValidationFindings returns a page of what the run concluded was wrong.
//
// GET /api/validations/{id}/findings
//
// Total is the size of the filtered set, not of the page.
func (c *Client) ValidationFindings(ctx context.Context, id string, q *FindingQuery) (*FindingPage, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "severity", q.Severity)
		setStr(qs, "status", q.Status)
		q.Page.apply(qs)
	}
	var out FindingPage
	if err := c.get(ctx, withQuery("/validations/"+esc(id)+"/findings", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidationEvidence returns a page of the evidence the run recorded.
//
// GET /api/validations/{id}/evidence
//
// Each record carries CheckID, which is what attaches an execution to the check
// that cited it. The Type filter is the one InvestigationEvidence has, over the
// same vocabulary.
func (c *Client) ValidationEvidence(ctx context.Context, id string, q *EvidenceQuery) (*EvidencePage, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "type", q.Type)
		q.Page.apply(qs)
	}
	var out EvidencePage
	if err := c.get(ctx, withQuery("/validations/"+esc(id)+"/evidence", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidationEvents returns a page of the validation's timeline. Same cursor
// semantics as InvestigationEvents.
//
// GET /api/validations/{id}/events
func (c *Client) ValidationEvents(ctx context.Context, id string, q *EventQuery) (*ValidationEventPage, error) {
	var out ValidationEventPage
	if err := c.get(ctx, withQuery("/validations/"+esc(id)+"/events", q.values()), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── organization ────────────────────────────────────────────────────────────

// GetOrganization returns the organisation this key speaks for, and what it
// holds.
//
// GET /api/organization
//
// On a deployment running CREDDA_AUTH=disabled this is a 404 with code
// NO_ORGANIZATION: the request names no organisation, and serving the first row
// in the table would be picking one. Use IsNotFound, or compare
// APIError.Code with CodeNoOrganization to tell that case apart from a deleted
// organisation.
func (c *Client) GetOrganization(ctx context.Context) (*OrganizationDetail, error) {
	var out OrganizationDetail
	if err := c.get(ctx, "/organization", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrganizationMembers returns a page of membership rows.
//
// GET /api/organization/members
//
// Read the doc comment on MemberPage before rendering an empty one.
func (c *Client) OrganizationMembers(ctx context.Context, p *Page) (*MemberPage, error) {
	qs := url.Values{}
	p.apply(qs)
	var out MemberPage
	if err := c.get(ctx, withQuery("/organization/members", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrganizationKeys returns the keys that can reach this organisation, revoked
// ones included.
//
// GET /api/organization/keys
//
// There is no companion that creates or revokes a key: the API has no such
// route. Keys are minted out of band by the operator.
func (c *Client) OrganizationKeys(ctx context.Context, p *Page) (*APIKeyPage, error) {
	qs := url.Values{}
	p.apply(qs)
	var out APIKeyPage
	if err := c.get(ctx, withQuery("/organization/keys", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── health and metrics ──────────────────────────────────────────────────────

// GetHealth returns readiness: every claim in it established by performing the
// thing it claims.
//
// GET /api/health
//
// A degraded engine answers 503. This method returns the Readiness anyway in
// that case, WITH a non-nil *APIError, because the body is the whole point of
// asking and a caller that only got an error would have to ask again to find
// out what failed. Check the error to know it is degraded; read the Readiness
// to know why.
//
// This route is behind the auth gate, because it names the schema version and
// what was checked. Liveness is not: see Livez.
//
// NEVER RETRIED, whatever WithRetries was set to. 503 is on the retryable list
// because it is what the engine answers for UNAVAILABLE and TOO_MANY_STREAMS,
// which are blips. Here it is not a blip, it is the answer: a readiness check
// failed, and the body says which. Asking a degraded database three more times
// on a rising backoff returns the same report a few seconds later, which is the
// one thing a caller reaching for a health check cannot afford. @credda/js has
// held this route out of its retry policy since its own rewrite; this client
// retried it, and the difference was an oversight rather than a decision.
func (c *Client) GetHealth(ctx context.Context) (*Readiness, error) {
	var out Readiness
	err := c.do(ctx, requestOptions{method: http.MethodGet, path: "/health", noRetry: true}, &out)
	if err == nil {
		return &out, nil
	}
	// 503 carries the full readiness body. Hand it back beside the error.
	if apiErr, ok := AsAPIError(err); ok && apiErr.StatusCode == http.StatusServiceUnavailable {
		var degraded Readiness
		if json.Unmarshal(apiErr.Body, &degraded) == nil && degraded.Status != "" {
			return &degraded, err
		}
	}
	return nil, err
}

// Livez reports whether the process is up and serving. It returns nil on
// success and an error otherwise.
//
// GET /livez
//
// Deliberately outside the auth gate and outside /api: it answers 204 with an
// empty body and discloses nothing — no schema version, no check names, no
// counts, no configuration. There is nothing here to protect, which is why it
// needs no credential and why a container healthcheck with no credential
// mechanism can use it. GetHealth is the one that establishes readiness.
func (c *Client) Livez(ctx context.Context) error {
	return c.do(ctx, requestOptions{method: http.MethodGet, path: "/livez", absolute: true}, nil)
}

// Metrics returns this process's Prometheus exposition, verbatim.
//
// GET /api/metrics
//
// It is behind the auth gate because of what it discloses — provider names,
// investigation counts, sandbox failures broken down by kind — and a Prometheus
// scrape_config can carry an Authorization header, so gating it costs nothing.
//
// WHAT THIS PROCESS ACTUALLY COUNTS: the registry is per-process and the API
// does not run investigations. Scraping this alone gives HTTP-shaped metrics
// and little else; the investigation, reproduction and model-usage counters
// increment in the worker. Scrape both.
func (c *Client) Metrics(ctx context.Context) (string, error) {
	var out string
	if err := c.get(ctx, "/metrics", &out); err != nil {
		return "", err
	}
	return out, nil
}
