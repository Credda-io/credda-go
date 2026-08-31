package credda

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Every method in this file is one route mounted in apps/api/src/app.ts. The
// path in each doc comment is the literal path the engine serves.
//
// All of them are GET except CreateInvestigation. That is not an omission: the
// API mounts exactly one POST route and no PATCH or DELETE at all. Resolution
// records are never revised, so the repository has no update method and the
// route module has no writing route to add one; keys are minted out of band by
// the operator, because nothing in the engine separates an OWNER from a VIEWER
// and a key that could mint keys would make every key a key factory.

// ── investigations ──────────────────────────────────────────────────────────

// InvestigationQuery filters ListInvestigations.
type InvestigationQuery struct {
	// State filters to one investigation state. Must be a member of
	// InvestigationStates; anything else is a 400 VALIDATION_FAILED.
	State string
	Page
}

// ListInvestigations returns a page of investigations, newest first.
//
// GET /api/investigations
//
// Total is the count of everything matching State in this organisation, so it
// does not shrink as you page.
func (c *Client) ListInvestigations(ctx context.Context, q *InvestigationQuery) (*InvestigationList, error) {
	qs := url.Values{}
	if q != nil {
		setStr(qs, "state", q.State)
		q.Page.apply(qs)
	}
	var out InvestigationList
	if err := c.get(ctx, withQuery("/investigations", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInvestigationInput is the body of CreateInvestigation.
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

// CreateInvestigation enqueues an investigation and returns it in state
// CREATED.
//
// POST /api/investigations
//
// It does NOT run anything. Execution is driven by the engine's worker, and
// this route only writes the row it will pick up. The returned detail therefore
// has no events, no evidence and no outcome; watch Stream or poll
// InvestigationEvents for what happens next.
//
// This call is never retried, even with WithRetries: the API accepts no
// idempotency key, so a repeat would enqueue a second run against the same
// report. The whole request body must be under 256KB or the engine refuses it
// unread with a 413.
func (c *Client) CreateInvestigation(ctx context.Context, in CreateInvestigationInput) (*InvestigationDetail, error) {
	var out InvestigationDetail
	if err := c.post(ctx, "/investigations", in, &out); err != nil {
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
func (c *Client) ValidationChecks(ctx context.Context, id string, p *Page) (*CheckPage, error) {
	qs := url.Values{}
	p.apply(qs)
	var out CheckPage
	if err := c.get(ctx, withQuery("/validations/"+esc(id)+"/checks", qs), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidationFindings returns a page of what the run concluded was wrong.
//
// GET /api/validations/{id}/findings
func (c *Client) ValidationFindings(ctx context.Context, id string, p *Page) (*FindingPage, error) {
	qs := url.Values{}
	p.apply(qs)
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
// that cited it.
func (c *Client) ValidationEvidence(ctx context.Context, id string, p *Page) (*EvidencePage, error) {
	qs := url.Values{}
	p.apply(qs)
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
