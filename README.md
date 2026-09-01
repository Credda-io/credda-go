<p align="center">
  <a href="https://credda.io">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/credda-lockup-white.png">
      <img alt="Credda" src="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/credda-lockup-black.png" width="480">
    </picture>
  </a>
</p>

# `credda`: official Go SDK for the Credda engine API

You label a bug report or a security vulnerability; Credda reproduces the
failure, diagnoses the cause, writes the patch, proves it with a test that fails
before and passes after, and hands back a diff. Whether that diff becomes a
pull request depends on which mechanism delivered it: the **GitHub App** path
opens one with no flag and no opt-in switch, for a run that reaches
`READY_FOR_REVIEW` with a proven verdict; the **GitHub Action**, which runs on
the caller's own runner, opens none unless its `open-pull-request` input is set,
and that input is declared on no version a caller can reach -- absent from
`action.yml` at the `v1` tag and on the action's default branch alike -- so
setting it today parses, runs green and delivers nothing. It proposes. It never
merges.

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

> **That command gets v0.3.0 today — checked 2026-08-30.** The highest tag this
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
		credda.WithBaseURL("https://credda.internal:4317"),
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
| `WithBaseURL(url)` | The engine's API root. There is no HOSTED default, because Credda runs on your own deployment; the local default is `http://localhost:4317`, the port `apps/api` binds. Supply this whenever the engine is anywhere else. |
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
except `CreateInvestigation`, `CreateInvestigationOnce` (the same route under an
idempotency key) and `CancelInvestigation` — the API mounts exactly
two `POST` routes and no `PATCH` or `DELETE` at all.

Every query parameter and body field this client sends is one the engine's
schemas declare, which is load-bearing rather than tidy: those schemas reject a
key they do not define with a **400 `VALIDATION_FAILED`** naming it, where an
unknown one used to be accepted and ignored.

### Investigations

| Method | Route |
| --- | --- |
| `ListInvestigations` | `GET /api/investigations` |
| `CreateInvestigation` | `POST /api/investigations` |
| `CreateInvestigationOnce` | `POST /api/investigations`, with `Idempotency-Key` |
| `CancelInvestigation` | `POST /api/investigations/{id}/cancel` |
| `GetInvestigation` | `GET /api/investigations/{id}` |
| `InvestigationEvents` | `GET /api/investigations/{id}/events` |
| `InvestigationEvidence` | `GET /api/investigations/{id}/evidence` |
| `StreamInvestigation` | `GET /api/investigations/{id}/stream` (SSE) |

#### Stopping a run

`CancelInvestigation` answers with what it **achieved**, and a `nil` error does
**not** mean the run stopped. Switch on `Status`:

| `Status` | HTTP | What is true |
| --- | --- | --- |
| `StatusCancelled` | 200 | The job was still queued and was refused its claim. **Nothing is running.** `State` is `CANCELLED`. |
| `StatusAlreadyCancelled` | 200 | It was already cancelled. Repeating the call is not an error. |
| `StatusCancellationRequested` | 202 | A worker is **inside** the run, holding a sandbox and possibly a model call. The request is durable and that worker honours it on its next heartbeat — but the run **has not stopped**, and the API writes no terminal state here. The run writes its own when it lets go; the event stream is how you learn that it did. |

```go
c, err := client.CancelInvestigation(ctx, id, credda.CancelInvestigationInput{
        Reason: "wrong repository",
})
if err != nil {
        return err
}
if c.Status == credda.StatusCancellationRequested {
        // Still running. Watch for the terminal state the run writes itself.
}
```

`Cancellation.Stopped()` is the short form. `Status` is a named type with
exported constants rather than a `bool` or a bare string, so a caller cannot
write `if c.Cancelled` and cannot compare against a literal that silently never
matches.

Two refusals come back as a 409 `*APIError`, because neither stopped anything:
`ALREADY_FINISHED` — the run reached a terminal state, so there is nothing to
stop and nothing to undo — and `NOT_CANCELLABLE` — the run is executing outside
the job queue, which is what `credda run` does, so the API cannot reach it and
will not pretend it did.

### Repositories

| Method | Route |
| --- | --- |
| `ListRepositories` | `GET /api/repositories` |
| `GetRepository` | `GET /api/repositories/{id}` |
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

- **No method advances a run**, and **no method asks the create route to start
  one** — which are two different statements, and this README used to make only
  the second-sounding version of the first. Advancing is the worker's and the
  API mounts no route for it. Starting is different: the engine's create body
  takes a `start` boolean (default `false`) that commits the investigation and
  its job in one write, and an optional downward-only `budget` beside it.
  `CreateInvestigationInput` carries **neither field**, so every create this
  package sends records a row and enqueues nothing, and
  `InvestigationDetail.Start` comes back `NOT_REQUESTED`. That is a gap in this
  client, not a limit of the engine. Stopping a run is a real route, added
  2026-08-29 — and even it cannot stop a run the job queue does not own.
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
call sends no idempotency key and is **never retried**, even with `WithRetries`
on: with no header the route does exactly what it always did, one run per
request, so a repeat would enqueue a second run against the same report.

### Enqueue one you can safely send twice

Opening an investigation commits a model budget, so a create repeated because a
socket died is a second bill. The route reads an **`Idempotency-Key`**:

| | HTTP | What is true |
| --- | --- | --- |
| First request | 201 → `StatusCreated` | A run was opened. |
| The same body again | 200 → `StatusReplayed` | The same run, returned again. **Nothing was created and nothing was billed.** |
| A **different** body | 409 `IDEMPOTENCY_KEY_REUSED` | Refused, and neither run is disclosed. Mint a new key for a new report. |
| No header at all | 201 | Exactly the old behaviour: one run per request. |

The claim is scoped to your organisation and never expires; it is deleted with
the investigation.

```go
req, err := credda.NewIdempotentCreate(credda.CreateInvestigationInput{
	RepositoryID: "repo_1",
	IssueTitle:   "Checkout returns 500 on submit",
	IssueBody:    report,
})
if err != nil {
	return err
}
if err := jobs.Record(ticketID, req.Key()); err != nil { // so a restart re-sends it
	return err
}

created, err := c.CreateInvestigationOnce(ctx, req)
if err != nil {
	return err
}
if created.Opened() {
	// This call opened the run.
} else {
	// StatusReplayed: an earlier attempt of ours got through. Nothing was billed.
}
```

`NewIdempotentCreate` mints the key and binds it to that body; the fields are
unexported, so a key cannot drift onto a report it does not stand for, and a new
report gets a new key. `IdempotentCreateWithKey` is for the other direction — a
restarted process re-sending a request under the key it recorded.

**This package never mints a key behind `CreateInvestigation`.** A key asserts
that two requests are one intent, and the engine's own handler is explicit that
only the caller knows that: re-running one report against a non-deterministic
engine is a real thing to want, and a body-derived key would make it impossible.
A key this library invented per call could also not be sent again by the process
that crashed and restarted — the case that actually double-bills.

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
	if ev.CompletedState != "" {
		fmt.Println("finished:", ev.CompletedState)
		break
	}
	fmt.Printf("[%d] %s: %s\n", ev.Sequence, ev.Type, ev.Event.Summary)
}
```

Keep the last `Sequence` you saw. Pass it as `StreamOptions.Since` to resume
without replaying — it goes out as both the `since` parameter and the
`Last-Event-ID` header.

A stream ends when the run reaches a terminal state — the engine sends a
`complete` frame, which arrives as a final item carrying `CompletedState` and no
`Event` — when you cancel `ctx`, when the engine drops it after five minutes
carrying nothing (`ErrStreamIdle`, and the run has *not* finished, so resuming
from the last `Sequence` is the answer), when the key is revoked
(`ErrStreamRevoked`), or on a transport failure. Debug-severity events are never
sent over a stream at all; use `InvestigationEvents` with `IncludeDebug` for
those.

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
`ALREADY_FINISHED`, `NOT_CANCELLABLE`, `IDEMPOTENCY_KEY_REUSED`,
`PAYLOAD_TOO_LARGE`, `UNAUTHENTICATED`, `TOO_MANY_STREAMS`, `UNAVAILABLE`,
`INTERNAL_ERROR`. Each is a `Code*` constant. `ALREADY_FINISHED` and
`NOT_CANCELLABLE` are the cancel route's and `IDEMPOTENCY_KEY_REUSED` is the
create route's, and they appear nowhere else. `UNAVAILABLE` is the one no response can actually carry — it is a
default every call site in the engine overrides — and it is a constant only so
that removing an exported name does not break a build.

`GetHealth` is the one method that returns **both** a value and an error: a
degraded engine answers 503 *and* names the check that failed, and throwing the
body away would force a second request for what you already asked.

### Retries

Off by default. `WithRetries(n)` retries network errors and 429/502/503/504 on
every `GET`, and on exactly one write: `CreateInvestigationOnce`, which carries
an `Idempotency-Key` the engine deduplicates against, so a repeat returns the run
the first attempt opened rather than opening a second. `CreateInvestigation` and
`CancelInvestigation` are never retried — the first carries no key, and the
second answers a different status when a repeat crosses a worker's heartbeat.

Backoff doubles from 300ms, capped at 5s, and the server's `Retry-After` wins
when one is sent. A cancelled context interrupts the wait, not just the request.

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

**Status, with a date on it — and it has moved.** This section used to say the
patch path was withheld from the default run pending the first model-backed run.
That expired on **2026-08-27**, when ADR 0019 put the Fix and Verify stages back
on the investigation path on the evidence that a model-backed provider exists
and works. The engine's `INVESTIGATION_STATES` carries the seven patch-path
states — `GENERATING_PATCH`, `TESTING_PATCH`, `VERIFYING`, `VERIFIED`,
`READY_FOR_REVIEW`, `VERIFICATION_FAILED`, `PATCH_REJECTED` — and
`credda.InvestigationStates` here withheld all seven until this commit, so
filtering `?state=READY_FOR_REVIEW` was a valid query this package told you did
not exist.

What is still conditional is the **deployment**, not the product. The gate is
not in the state graph: `provider.isGenerative` in the orchestrator decides
whether a run enters the fix stage at all, because a rule-based provider cannot
author a patch and a heuristic patch is worse than none. On a deployment that
resolved no generative provider, `InvestigationDetail.Patches` and
`.Verifications` come back empty and `Resolution.Fix` and `.Verification` come
back `nil`, with the reason named in `Confidence.NotEstablished` rather than
papered over. That is a fact about one deployment's configuration, and it must
not be written down again as a fact about Credda.

ADR 0018 is the authority and says it plainly: a sentence describing a missing
capability must be falsifiable by a number, and must move when the number moves.
This section is what happens when nobody moves it.

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
