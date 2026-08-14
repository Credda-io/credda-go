// Trust Gateway and Trust Actions.
//
// A DIFFERENT product from the threshold policies in parity.go, and the names are
// close enough to be worth stating: a threshold policy watches ONE condition
// edge-triggered over time, while a Trust Gateway policy is a whole statement of
// what the CALLER requires ("score >= 80 and at least 3 verifying platforms"),
// answered as one question. They share the evidence and the evidence/decision
// boundary, not a shape, and they sit at different mounts.
//
// THREE-VALUED, AND THE THIRD VALUE IS THE POINT. TrustResultInsufficientEvidence
// is NOT TrustResultNotSatisfied. A subject nobody has measured yet has not failed
// the policy, and code that collapses the two makes Credda assert a negative about
// every new subject on the network. Switch on all three.
//
// The score scale is 0-100, always. A rule written against a 0-1000 model is
// refused at authoring time rather than stored as one no subject could satisfy.
//
// Its own file rather than more of parity.go: that file mirrors the TypeScript
// client's pre-existing surface, and this is a distinct product with its own
// vocabulary.

package credda

import (
	"context"
	"net/url"
)

// The three results a policy evaluation can produce. They never collapse into two.
const (
	TrustResultSatisfied            = "satisfied"
	TrustResultNotSatisfied         = "not_satisfied"
	TrustResultInsufficientEvidence = "insufficient_evidence"
)

// TrustRule is one leaf comparison. Value is omitted for the EXISTS operator,
// which asks only whether the signal was measured at all.
type TrustRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

// CreateTrustPolicyInput creates a Trust Gateway policy. Rules is a rule tree: a
// leaf, or a map carrying a non-empty "all" or "any". An empty group is REJECTED
// rather than treated as vacuously true, which is the classic default-allow hole.
type CreateTrustPolicyInput struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose,omitempty"`
	Rules   any    `json:"rules"`
}

// UpdateTrustPolicyInput renames, enables/disables, or publishes a new version.
// Supplying Rules APPENDS a version; a stored definition is never rewritten.
type UpdateTrustPolicyInput struct {
	Name     *string `json:"name,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
	Rules    any     `json:"rules,omitempty"`
}

// TrustPolicy is one Trust Gateway policy. Rules is present on create, read and
// update, and omitted from list rows.
type TrustPolicy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	Version   int    `json:"version"`
	IsActive  bool   `json:"isActive"`
	Livemode  bool   `json:"livemode"`
	Rules     any    `json:"rules,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// TrustPolicyListResult is the cursor-paginated payload from ListTrustPolicies.
type TrustPolicyListResult struct {
	Data       []TrustPolicy `json:"data"`
	NextCursor *string       `json:"nextCursor"`
}

// EvaluateTrustPolicyInput names a subject and exactly one of Policy (a stored
// id) or Rules (a one-off document).
type EvaluateTrustPolicyInput struct {
	Subject string `json:"subject"`
	Policy  string `json:"policy,omitempty"`
	Rules   any    `json:"rules,omitempty"`
	Explain *bool  `json:"explain,omitempty"`
}

// TrustRuleCheck is one rule and how it fared: the CALLER's own threshold plus a
// per-rule verdict, never the subject's measured value.
type TrustRuleCheck struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	Result   string `json:"result"`
}

// TrustPolicyRef names the exact policy version that produced a result.
type TrustPolicyRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// TrustSubjectRef carries the caller's own subject reference, never a stored id.
//
// On a watch and on a trust.policy.result_changed event, an externalId is echoed
// verbatim but a SHARE TOKEN is masked (prefix plus last four characters): a live
// token is a bearer capability, and those surfaces travel into delivery logs and
// notification channels. Map on the watch id, not on this field.
type TrustSubjectRef struct {
	Reference string `json:"reference"`
}

// TrustEvaluation is the answer to one policy question. Policy is nil when the
// policy was supplied inline; Checks is present only when Explain was requested.
type TrustEvaluation struct {
	Result      string           `json:"result"`
	Policy      *TrustPolicyRef  `json:"policy,omitempty"`
	Subject     TrustSubjectRef  `json:"subject"`
	EvaluatedAt string           `json:"evaluatedAt"`
	Livemode    bool             `json:"livemode"`
	Checks      []TrustRuleCheck `json:"checks,omitempty"`
	// Note restates the evidence/decision boundary on every answer.
	Note string `json:"note"`
}

// TrustWatch is a continuous evaluation of one subject against one policy.
type TrustWatch struct {
	ID      string          `json:"id"`
	Policy  string          `json:"policy"`
	Subject TrustSubjectRef `json:"subject"`
	// AuthorizationBasis is share_token or platform_relationship. Descriptive:
	// the live check is a re-resolution on every evaluation, not this field.
	AuthorizationBasis string `json:"authorizationBasis"`
	// LastResult is the answer last DELIVERED for this watch, nil before the first
	// evaluation. NOT the current answer: it is what the caller's endpoint was last
	// told, which is what makes it useful for reconciling a missed delivery.
	//
	// A POINTER on purpose. Decoded into a plain string, a JSON null becomes "",
	// and "nothing has been delivered yet" would be indistinguishable from a
	// delivered result. That substitution is the defect SDK_NULL_PARITY.md exists
	// for.
	LastResult   *string `json:"lastResult"`
	LastResultAt *string `json:"lastResultAt"`
	IsActive     bool    `json:"isActive"`
	// RevokedAt is set when an evaluation found the authorization no longer held
	// (a revoked share token, or a relationship that no longer exists) and stopped
	// the watch. This is why a watch went quiet.
	RevokedAt *string `json:"revokedAt"`
	Livemode  bool    `json:"livemode"`
	CreatedAt string  `json:"createdAt"`
	Note      string  `json:"note,omitempty"`
}

// TrustWatchListResult is the cursor-paginated payload from ListTrustWatches.
type TrustWatchListResult struct {
	Data       []TrustWatch `json:"data"`
	NextCursor *string      `json:"nextCursor"`
}

// CreateTrustPolicy stores a versioned statement of what the caller requires.
// POST /api/v1/trust/policies.
//
// Purpose is descriptive and never changes evaluation. Employment, tenancy,
// lending, credit and insurance contexts are REFUSED with PURPOSE_NOT_PERMITTED:
// those are regulated consumer-reporting uses and Credda is not a consumer
// reporting agency.
func (c *Client) CreateTrustPolicy(ctx context.Context, input CreateTrustPolicyInput) (*TrustPolicy, error) {
	var out TrustPolicy
	if err := c.post(ctx, "/trust/policies", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTrustPolicies lists the caller's Trust Gateway policies, cursor-paginated.
// GET /api/v1/trust/policies.
func (c *Client) ListTrustPolicies(ctx context.Context, limit *int, cursor string) (*TrustPolicyListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out TrustPolicyListResult
	if err := c.get(ctx, withQuery("/trust/policies", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTrustPolicy reads one policy, at its current version or a past one. Pass a
// non-nil version to recover the exact rules behind a result delivered earlier.
// GET /api/v1/trust/policies/:id.
func (c *Client) GetTrustPolicy(ctx context.Context, id string, version *int) (*TrustPolicy, error) {
	qs := url.Values{}
	setInt(qs, "version", version)
	var out TrustPolicy
	if err := c.get(ctx, withQuery("/trust/policies/"+esc(id), qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTrustPolicy renames, enables/disables, or publishes a new version.
// PATCH /api/v1/trust/policies/:id.
func (c *Client) UpdateTrustPolicy(ctx context.Context, id string, patch UpdateTrustPolicyInput) (*TrustPolicy, error) {
	var out TrustPolicy
	if err := c.patch(ctx, "/trust/policies/"+esc(id), patch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EvaluateTrustPolicy answers whether a subject's permitted evidence satisfies a
// policy. POST /api/v1/trust/evaluate.
//
// Name the subject one of exactly two ways, both of which already carry
// authorization: a share token the subject minted and can revoke (crd_share_...),
// or an externalId the caller has an existing relationship with. There is
// deliberately no lookup by email or name, and an unknown subject and an
// unauthorized one both answer 404, so this cannot be used to test whether a
// Credda id exists.
func (c *Client) EvaluateTrustPolicy(ctx context.Context, input EvaluateTrustPolicyInput) (*TrustEvaluation, error) {
	var out TrustEvaluation
	if err := c.post(ctx, "/trust/evaluate", input, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateTrustWatch watches a subject against a stored policy (Trust Actions).
// POST /api/v1/trust/watches.
//
// The asynchronous form of EvaluateTrustPolicy: the same question asked again
// whenever the subject's evidence moves, delivering trust.policy.result_changed
// to the caller's webhooks when the ANSWER changes. The first evaluation arrives
// with a nil previous; an unchanged answer sends nothing.
//
// IDEMPOTENT on (policy, subject), so a retried call returns the same watch rather
// than a second one that would double every future delivery.
//
// Only a stored, versioned policy can be watched: a watch fires unattended for
// months, and the definition behind any result must stay recoverable.
//
// Authorization is RE-CHECKED on every evaluation, so a share token the subject
// revokes stops the watch, and the row then carries RevokedAt.
func (c *Client) CreateTrustWatch(ctx context.Context, subject, policy string) (*TrustWatch, error) {
	var out TrustWatch
	body := map[string]string{"subject": subject, "policy": policy}
	if err := c.post(ctx, "/trust/watches", body, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTrustWatches lists the caller's watches, cursor-paginated.
// GET /api/v1/trust/watches.
//
// Each row carries LastResult, the answer last DELIVERED for it, so a missed
// webhook can be reconciled without re-evaluating.
func (c *Client) ListTrustWatches(ctx context.Context, limit *int, cursor string) (*TrustWatchListResult, error) {
	qs := url.Values{}
	setInt(qs, "limit", limit)
	setStr(qs, "cursor", cursor)
	var out TrustWatchListResult
	if err := c.get(ctx, withQuery("/trust/watches", qs), true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTrustWatch stops watching. DELETE /api/v1/trust/watches/:id.
func (c *Client) DeleteTrustWatch(ctx context.Context, id string) error {
	return c.delete(ctx, "/trust/watches/"+esc(id))
}
