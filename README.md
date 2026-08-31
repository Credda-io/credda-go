<p align="center">
  <a href="https://credda.io">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/credda-lockup-white.png">
      <img alt="Credda" src="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/credda-lockup-black.png" width="480">
    </picture>
  </a>
</p>

# `credda`: official Go SDK for the Credda engine API

Credda finds the defects and security vulnerabilities in your production and QA
environments, reproduces the failure, diagnoses the cause, writes the patch,
proves it with a test that fails before and passes after, and opens a pull
request. It proposes. It never merges.

This package is a typed Go client over that engine's HTTP API: read the queue,
read what a run established, watch a run happen live, and enqueue an
investigation from a bug report. That API is documented at
[api.credda.io/reference](https://api.credda.io/reference).

**Standard library only:** `net/http`, `encoding/json`, `bufio`. No third-party
modules. `go.mod` has no `require` block, and
[a test fails the build if one appears](#the-no-dependencies-rule).

---

> ### ⚠️ This module changed meaning at v0.4.0
>
> Tags **v0.1.1, v0.2.0 and v0.3.0** of this import path are a client for a
> different product: Credda's retired 0–100 reliability-score API — trust
> scores, share tokens, earnings, disputes, webhooks. None of it exists any
> more, and none of it is in this package.
>
> If you are on v0.3.0 or earlier, nothing you call here survives. Read
> [Versioning](#versioning) before upgrading.

---

## Install

```sh
go get github.com/Credda-io/credda-go
```

```go
import credda "github.com/Credda-io/credda-go"
```

That is the whole install. No vendoring, no `replace` directive, no transitive
graph to audit.

> **That command gets v0.3.0 today — checked 2026-08-28.** The highest tag this
> module has is **v0.3.0**, which is the retired reliability-score client. The
> engine client this README documents is **v0.4.0, and it is not tagged yet**;
> `proxy.golang.org` lists only `v0.1.0`, `v0.1.1`, `v0.2.0`, `v0.3.0`. Until
> v0.4.0 is cut, `go get` resolves to the wrong product, and the `retract`
> directives described under [Versioning](#versioning) — which are already in
> `go.mod` — have no effect, because the proxy only honours a `retract` from a
> published version.
> To use the code in this repository now, `go get` it at a commit
> (`go get github.com/Credda-io/credda-go@main`).

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	credda "github.com/Credda-io/credda-go"
)

func main() {
	c := credda.NewClient(
		credda.WithBaseURL("https://credda.internal:3001"),
		credda.WithAPIKey(os.Getenv("CREDDA_API_KEY")),
		credda.WithRetries(2),
	)
	ctx := context.Background()

	// Every record this engine has produced that nothing verified.
	unverified, err := c.ListResolutions(ctx, &credda.ResolutionQuery{
		Confidence: "NOT_ESTABLISHED",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d of them\n", unverified.Total)
	for _, r := range unverified.Resolutions {
		fmt.Printf("\n%s — %s\n", r.ID, r.Reported)
		for _, gap := range r.NotEstablished {
			fmt.Printf("  not established: %s\n", gap)
		}
	}
}
```

That query — every record with `NOT_ESTABLISHED` confidence — is the one that
keeps this product honest with itself, which is why it is the example.

## Configuration

| Option | What it does |
| --- | --- |
| `WithBaseURL(url)` | The engine's API root. **Required in practice**: Credda runs on your own deployment, so there is no hosted default. The default is `http://localhost:3001`. |
| `WithAPIKey(key)` | The organisation API key, sent as an RFC 6750 bearer token on every request. |
| `WithHTTPClient(hc)` | Your own `*http.Client` — timeouts, transport, proxies. Needed for streaming; see below. |
| `WithRetries(n)` | Opt-in retries of transient failures. Off by default. |
| `WithRetryBackoff(base, max)` | First wait and the ceiling on any single wait. Defaults 300ms and 5s. |

### Authentication

Your deployment sets `CREDDA_AUTH` to `enforced` or `disabled`. There is no third
value — "enforce if a key happens to exist" is the mode that lets a deployment
believe it is protected while a missing row quietly opens it.

- **`enforced`**: every `/api` route needs the bearer token. Without it you get
  a 401 `UNAUTHENTICATED`.
- **`disabled`**: the request carries no organisation. Reads work; the three
  `/api/organization` routes answer **404 `NO_ORGANIZATION`** rather than
  guessing which organisation you meant. A 404 and not a 401, because the
  deployment did not ask for credentials — what is missing is the resource "the
  current organisation", which does not exist for a request like that.

`Livez` needs no credential in either mode.

The key names an *organisation*, never a person. `api_keys` has an `org_id` and
no `user_id`, which is why `OrganizationMember.RoleEnforced` is `false` and why
there is no route here that mints or revokes a key.

## What this client can do

Every method is one route the engine actually mounts. All of them are `GET`
except `CreateInvestigation` — the API mounts exactly one `POST` and no `PATCH`
or `DELETE` at all.

### Investigations

| Method | Route |
| --- | --- |
| `ListInvestigations` | `GET /api/investigations` |
| `CreateInvestigation` | `POST /api/investigations` |
| `GetInvestigation` | `GET /api/investigations/{id}` |
| `InvestigationEvents` | `GET /api/investigations/{id}/events` |
| `InvestigationEvidence` | `GET /api/investigations/{id}/evidence` |
| `StreamInvestigation` | `GET /api/investigations/{id}/stream` (SSE) |

### Repositories

| Method | Route |
| --- | --- |
| `ListRepositories` | `GET /api/repositories` |
| `RepositoryLearnings` | `GET /api/repositories/{id}/learnings` |

### Resolutions

| Method | Route |
| --- | --- |
| `ListResolutions` | `GET /api/resolutions` |
| `LatestResolution` | `GET /api/resolutions/latest?investigation={id}` |
| `GetResolution` | `GET /api/resolutions/{id}` |

### Validations

| Method | Route |
| --- | --- |
| `ListValidations` | `GET /api/validations` |
| `GetValidation` | `GET /api/validations/{id}` |
| `ValidationChecks` | `GET /api/validations/{id}/checks` |
| `ValidationFindings` | `GET /api/validations/{id}/findings` |
| `ValidationEvidence` | `GET /api/validations/{id}/evidence` |
| `ValidationEvents` | `GET /api/validations/{id}/events` |
| `StreamValidation` | `GET /api/validations/{id}/stream` (SSE) |

### Organization, health, metrics

| Method | Route |
| --- | --- |
| `GetOrganization` | `GET /api/organization` |
| `OrganizationMembers` | `GET /api/organization/members` |
| `OrganizationKeys` | `GET /api/organization/keys` |
| `GetHealth` | `GET /api/health` |
| `Metrics` | `GET /api/metrics` |
| `Livez` | `GET /livez` |

### What is deliberately not here

Nothing in this package invents an endpoint, a field, a parameter or a
behaviour. If the engine does not serve it, it is not here. In particular:

- **No method starts, cancels or advances a run.** `CreateInvestigation` writes
  a row in state `CREATED` and returns; execution is driven by the engine's
  worker, and the API exposes no route for it.
- **No method creates a validation, a patch, or a pull request.** Those are the
  worker's, and the API has no route for them.
- **No method mints or revokes an API key.** The API has none. Nothing in the
  engine separates an `OWNER` from a `VIEWER`, so a key that could mint keys
  would make every key a key factory, and revoking the one that leaked would not
  revoke what it issued.
- **No method updates a resolution.** Records are assembled from what a run
  executed and are never revised. The table has no update path and neither does
  this client.
- **No webhook verification.** The previous version of this package shipped HMAC
  webhook-signature verification for the trust API. The engine API delivers no
  webhooks, so `crypto/hmac` is gone from the dependency list above along with
  the code that used it.

## Reading a run

### Enqueue an investigation

```go
detail, err := c.CreateInvestigation(ctx, credda.CreateInvestigationInput{
	RepositoryID: "repo_1",
	IssueTitle:   "Checkout returns 500 on submit",
	IssueBody:    "Since Tuesday, POST /checkout 500s for cart totals over 1000.",
	IssueRef:     credda.String("https://tracker.example/ISSUE-41"),
})
// detail.Investigation.State == "CREATED". Nothing has run yet.
```

The body must be under 256KB or the engine refuses it unread with a 413. This
call is **never retried**, even with `WithRetries` on: the API accepts no
idempotency key, so a repeat would enqueue a second run against the same report.

### Watch it happen

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel() // ALWAYS. An abandoned stream holds a connection on the engine.

c := credda.NewClient(
	credda.WithBaseURL(base),
	credda.WithAPIKey(key),
	credda.WithHTTPClient(&http.Client{}), // no timeout: the default 30s cuts a healthy stream
)

events, err := c.StreamInvestigation(ctx, "inv_1", &credda.StreamOptions{Since: 0})
if err != nil {
	log.Fatal(err) // e.g. 503 TOO_MANY_STREAMS: the engine's budget is 64 concurrent
}

for ev := range events {
	if ev.Err != nil {
		if errors.Is(ev.Err, credda.ErrStreamRevoked) {
			log.Fatal("this API key was revoked; reconnecting will not help")
		}
		log.Fatal(ev.Err)
	}
	fmt.Printf("[%d] %s: %s\n", ev.Sequence, ev.Type, ev.Event.Summary)
}
```

Keep the last `Sequence` you saw. Pass it as `StreamOptions.Since` to resume
without replaying — it goes out as both the `since` parameter and the
`Last-Event-ID` header.

A stream ends when you cancel `ctx`, when the engine drops it after five minutes
carrying nothing, when the key is revoked, or on a transport failure. Debug-
severity events are never sent over a stream at all; use `InvestigationEvents`
with `IncludeDebug` for those.

### Read what it established

```go
r, err := c.GetResolution(ctx, "res_1")

if r.Fix == nil {
	// No patch was written. This is nil, not an empty ResolutionFix — see below.
}
if r.Verification != nil {
	s := r.Verification.Signals
	// The claim, in four fields: the demonstrated failure is gone, and a test
	// that failed before now passes.
	fmt.Println(s.ReproductionBefore, "->", s.ReproductionAfter)   // FAIL -> PASS
	fmt.Println(s.RegressionTestBefore, "->", s.RegressionTestAfter) // FAIL -> PASS
}
for _, gap := range r.Confidence.NotEstablished {
	fmt.Println("not established:", gap)
}
```

## Two rules this client is built around

### Unmeasured is not zero

`encoding/json` decodes a JSON `null` into a non-pointer field as a **no-op**:
the field keeps its zero value and no error is returned. Nothing fails, nothing
warns, and a value the engine deliberately did not measure arrives as a
confident number.

The engine is careful about this and this client must not undo it. So **every
nullable field is a pointer**, and `nil` is the absence surviving the trip:

| Field | `nil` means | Zero means |
| --- | --- | --- |
| `Investigation.Outcome` | the run has not finished | — |
| `ResolutionSummary.VerificationVerdict` | nothing verified this | — |
| `Resolution.Fix` | no patch was written | — |
| `ResolutionRepro.TimedOutAttempts` | this record does not say | nothing was killed |
| `ValidationSummary.ExecutableFilesChanged` | the change was never analysed | analysed, touched nothing executable — a **success** |
| `ValidationCheck.BaseStatus` | the base commit was never re-run | — |
| `Readiness.SchemaVersion` | the version could not be read | an unmigrated database |
| `APIKey.LastUsedAt` | never used | — |

Read the last two columns of that table again before rendering any of these. A
`0` that means "we checked and the answer is zero" is a real answer and must
stay visible; a `0` that came from a `null` is a claim nobody made.

### Some facts are written twice, and neither half can be read alone

| Pair | Why |
| --- | --- |
| `Confidence.Class` + `Confidence.NotEstablished` | A class with its gaps a request away is exactly the bare assertion the pairing prevents. `NotEstablished` is empty *only* when the class is `ESTABLISHED`. |
| `Member.Role` + `Member.RoleEnforced` | `RoleEnforced` is `false`. A Role column rendered without it is a rendered access model, and this product has none. |
| `Verification.Verdict` + `Verification.Signals` | The verdict is a mechanical function of the signals, never a model's opinion. Showing it alone throws away what makes it checkable. |
| `Validation.State` + `Validation.Outcome` | The state says who stopped; the outcome says what was concluded. `COMPLETED` carries no judgement. |
| `Check.Status` + `Check.BaseStatus` | `FAILED` + base `PASSED` means *this change caused it*. `FAILED` + base `FAILED` means it was already broken. One field cannot say both. |

There are no percentages anywhere in this API and none in this client.
`Confidence`, `EvidenceStrength` and `FindingConfidence` are ordinal labels
because Credda has no calibrated probability model, and the field a reviewer
reads to decide whether to trust a fix is the worst possible place to invent a
number.

## Errors

Every non-2xx response is an `*APIError`.

```go
out, err := c.GetInvestigation(ctx, id)
if credda.IsNotFound(err) {
	// No such id — or it belongs to another organisation. The API does not
	// distinguish those, deliberately.
}
if apiErr, ok := credda.AsAPIError(err); ok {
	log.Printf("%s (%d) requestId=%s", apiErr.Code, apiErr.StatusCode, apiErr.RequestID)
}
```

**Always log `RequestID`.** A 500 from the engine carries no detail on purpose —
stack traces and SQL text never cross the boundary — and the `X-Request-Id` is
the only thing that finds the failure in the engine's logs.

Codes: `INVALID_REQUEST`, `VALIDATION_FAILED`, `NOT_FOUND`, `NO_ORGANIZATION`,
`PAYLOAD_TOO_LARGE`, `UNAUTHENTICATED`, `TOO_MANY_STREAMS`, `UNAVAILABLE`,
`INTERNAL_ERROR`. Each is a `Code*` constant.

`GetHealth` is the one method that returns **both** a value and an error: a
degraded engine answers 503 *and* names the check that failed, and throwing the
body away would force a second request for what you already asked.

### Retries

Off by default. `WithRetries(n)` retries network errors and 429/502/503/504 —
on `GET` only. Backoff doubles from 300ms, capped at 5s, and the server's
`Retry-After` wins when one is sent. A cancelled context interrupts the wait,
not just the request.

`GetHealth` is held out of this, whatever `n` is. Its 503 is the answer rather
than a blip — a readiness check failed and the body names which — so repeating
it returns the same report a backoff later. Every other `GET` retries normally.
`CreateInvestigation` is never retried either, for the idempotency reason above.

This is the same list, precedence, ceiling and set of exclusions as
[`@credda/js`](https://github.com/Credda-io/credda-js).

## Versioning

**The module path does not change. It stays `github.com/Credda-io/credda-go`.**

Go's import-path versioning rule — the one that requires a `/v2` suffix — applies
from major version **2 onward**. This module has never left v0. Under SemVer and
under Go's own module reference, a v0 module makes no compatibility promise at
all, and a breaking change between v0 minors is exactly what v0 is for. Moving
to `/v2` would be wrong twice over: it would claim a v2 that does not exist, and
it would strand the v0.3.0 tags on a path that then has no v1.

So the concrete plan is:

1. **The pivot ships as `v0.4.0`** on the existing path. Same import line, an
   entirely different package behind it. The banner at the top of this file, and
   this section, are how a reader finds that out before it costs them anything.
2. **v0.1.1, v0.2.0 and v0.3.0 get `retract` directives** in `go.mod`, published
   with v0.4.0:

   ```
   retract (
       v0.1.1 // Client for the retired reliability-score API. No route it calls exists.
       v0.2.0 // Same.
       v0.3.0 // Same.
   )
   ```

   `retract` is not a `require`; it adds no dependency and the standard-library-
   only property is untouched. What it does: `go get -u` stops selecting them,
   and `go list -m -versions` shows them withdrawn with the reason.

   **What it does not do**, and this is the honest caveat: retraction does not
   delete anything from the module proxy. Code that already pins v0.3.0 keeps
   building forever, against a client for an API that no longer answers. That is
   the correct outcome — breaking someone's build to make a point is worse — but
   it means the tags stay reachable and this README is the only thing that tells
   a reader what they are.
3. **Pre-1.0 continues.** v0.x may break again while the engine API is still
   moving. Pin an exact version.
4. **v1.0.0 is not on a date.** It is on a condition: the engine's API surface
   stable enough that a breaking change to it would be a considered decision
   rather than a Tuesday. When that holds, v1.0.0 on this same path — still no
   `/v2`, because v1 does not take a suffix either.

### What changed, plainly

| Retired at v0.3.0 | Replaced by |
| --- | --- |
| `TrustGateway*`, trust policies, watches | nothing — the product does not exist |
| `GetScore`, `ResolveToken`, share tokens | nothing |
| `GetEarnings`, earnings windows, stability | nothing |
| `ReportEvent`, disputes, confirmations | nothing |
| `VerifyWebhook` and `crypto/hmac` | nothing — the engine API delivers no webhooks |
| `/api/v1/*`, `api.credda.io` | `/api/*` on your own deployment |
| `crd_live_…` keys, public vs platform routes | one organisation key, one deployment-wide gate |

Kept, because it was product-neutral: the retry policy, the `Retry-After`
handling, the option pattern, and the error-wrapping shape.

## Where the fix is today

Credda's promise is the pull request. Writing the patch, proving it, and opening
the PR is what the product is for, and this client types the whole record that
describes one: `Patch`, `Verification`, `VerificationSignals`,
`Resolution.Fix`, `Resolution.RegressionProtection`.

**Status, with a date on it:** as of the API this client was written against
(August 2026), the engine's patch path is withheld from the default run pending
the first model-backed run. On such a deployment `InvestigationDetail.Patches`
and `.Verifications` come back empty, and `Resolution.Fix` and
`.Verification` come back `nil`, with the reason named in
`Confidence.NotEstablished` rather than papered over.

That is a status and not a principle. It is gated on one API key, it moves when
the number moves, and this client already types what it will carry when it does.
The Credda engine's own [ADR 0018](https://github.com/Credda-io/core) is the
authority on this and says it plainly: a sentence describing a missing capability
must be falsifiable by a number, and must move when the number moves.

## The no-dependencies rule

`go.mod` has no `require` block and no `replace`. This is not a stylistic
preference: this package is compiled into your build, and every dependency it
takes is a dependency you take — in a product whose entire argument is that its
output can be audited.

`TestModuleHasNoThirdPartyDependencies` reads `go.mod` from disk and fails if a
`require` or `replace` line appears. Adding a dependency here is a change to what
this README promises, and it fails a test rather than passing a review.

## Development

```sh
gofmt -l .      # must print nothing
go vet ./...
go test ./...
go test -cover ./...
```

CI runs all four on Go 1.21, plus a gitleaks scan over the full history.

## License

MIT. See [LICENSE](LICENSE).
