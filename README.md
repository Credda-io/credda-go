<p align="center">
  <a href="https://credda.io">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/creddalockuplongdarktransparent.png">
      <img alt="Credda" src="https://raw.githubusercontent.com/Credda-io/credda-go/main/assets/creddalockuplonglighttransparent.png" width="360">
    </picture>
  </a>
</p>

# `credda`: official Go SDK for the Credda API

A dependency-free Go client for the [Credda](https://credda.io) Reliability Score
API. It mirrors the TypeScript SDK (`@credda/js`) endpoint for endpoint: public
share-token reads, platform-key scored reads, event ingestion, share tokens,
dispute resolution, webhook management, and inbound webhook verification.

Standard library only: `net/http`, `crypto/hmac`, `encoding/json`. No third-party
modules.

## Install

```sh
go get github.com/Credda-io/credda-go
```

```go
import credda "github.com/Credda-io/credda-go"
```

That is the whole install. No credentials, no vendoring, no `replace` directive.

Everything below assumes the package is imported as `credda`.

## Quickstart

### Public: resolve a share token (no API key)

```go
client := credda.NewClient()

trust, err := client.ResolveToken(ctx, "crd_share_…")
if err != nil {
    log.Fatal(err)
}
fmt.Println(trust.FinalScore, trust.ScoreBand)
```

`ResolveToken`, `GetTrustExport`, `GetDIDDocument` and `GetTrustRegistry` are the
only methods that work without a key. They are safe for untrusted contexts.

### Platform: read a score (API key required)

```go
client := credda.NewClient(
    credda.WithAPIKey(os.Getenv("CREDDA_API_KEY")),
)

score, err := client.GetScore(ctx, "your-platform-user-id")
if err != nil {
    var apiErr *credda.APIError
    if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
        // no score yet for that user
    }
    return err
}
fmt.Println(score.FinalScore, score.Breakdown.CR)
```

> A `crd_live_…` key is server-side only. Never embed one in a client binary or
> browser bundle. Use a share token there.

### ⚠️ Unmeasured is not zero

The single most important rule when reading anything score-shaped out of this
SDK. Credda distinguishes **"we measured it and it was bad"** from **"there was
nothing to measure"**, and the API says which on the wire. Go makes this easy to
get wrong, so read this before you render a number.

`encoding/json` decodes a JSON `null` into a non-pointer `float64` as a **no-op**:
the field keeps its zero value and **no error is returned**. A rate the API never
measured would arrive in your struct as `0.0`, and in a product whose bands run
down to *At Risk* a `0.0` is the worst possible record, not an unknown one.

**Every nullable score, band and rate is therefore a pointer.** `nil` means the
API sent `null`, which means *not measured*. A genuinely measured `0` is a
non-nil pointer to `0`, and it must still be displayed. The compiler will not let
you read one without deciding what the absence means:

```go
comps, err := client.GetScoreComponents(ctx, "user-42")
if err != nil {
    return err
}

// The whole-payload state first: is there anything to measure at all?
if comps.DataSufficiency != nil && comps.DataSufficiency.InsufficientData {
    fmt.Println(comps.DataSufficiency.Note) // safe to show verbatim
}

for _, c := range comps.Components {
    if c.Score == nil {
        fmt.Printf("%-16s not measured\n", c.Label) // NEVER "0"
        continue
    }
    fmt.Printf("%-16s %.0f\n", c.Label, *c.Score) // 0 here is a REAL 0
}
```

The nullable values, and the discriminator that says *why* each is absent:

| Read | Nil-checked values | Discriminator explaining the absence |
|---|---|---|
| `GetScoreComponents` | `ScoreComponent.Score` | `ScoreComponent.Available`, `ScoreComponentsPayload.DataSufficiency` |
| `GetScoreExplain` | (`Value`, `Contribution` are still `float64`, see below) | `ScoreExplainFactor.Available`, `ScoreExplainPayload.DataSufficiency` |
| `GetScoreExplain().ReasonCodes` | `FinalScore` | `InsufficientData`, `DataState` |
| `GetTrustSummary` | `Evidence.CompletionRate`, `Evidence.OnTimeRate` | `TrustSummaryEvidence.InsufficientData` |
| `GetProfessionalRecord` | `Reliability.Score`, `Reliability.Band` | `VerifiedExperience`, `Tenure` |
| `GetReliabilityReport` | `Reliability.Score`, `Reliability.Band`, `Metrics.CompletionRate`, `Metrics.OnTimeRate`, `Metrics.Consistency`, `Metrics.DisputeRate`, `Metrics.Recency` | `Metrics.InsufficientData`, `Metrics.DataState` |
| `GetDispatchReliability` | `Score`, `Band`, `NoShowRate`, `OnTimeRate`, `DaysSinceLastEvent` | `Evidence.TotalOutcomes` |
| `ProjectScore` / `AnalyzeDocument` | none | `Timeliness.ProjectionIsUpperBound` (`Projected` is a best case, not a point estimate) |

Four things follow from this, and each has a test in `unmeasured_test.go`:

- **A measured zero is still real and must still be shown.** A non-nil pointer to
  `0` with `Available: true` is a record that genuinely completed nothing. Hiding
  real bad news is a worse failure than the substitution this rule exists to
  prevent. Never collapse `nil` and `0` back together on the way to your UI.
- **A nil band is not an empty band.** `Reliability.Band` is `*string`, so an
  unscored subject is `nil`, not `""`. Do not feed `""` into a band switch: the
  default arm is usually the floor of the scale, which is the exact substitution
  this rule forbids.
- **An absent measurement is never an adverse reason.** When
  `ReasonCodes.InsufficientData` is true, `AdverseActionReasons` and
  `SupportingFactors` are **empty by construction**, and the one code present is
  `informational`. Never draw a Regulation B statement of specific reasons from
  it. `DataState` says whether the record has no outcomes at all
  (`no_recorded_outcomes`) or outcomes whose score has not been computed yet
  (`score_not_yet_computed`).
- **An older API is safe.** The discriminators are additive; against a server that
  predates them the flag is Go's zero value and the pointer blocks are `nil`.
  Treat an absent flag as *unknown*, not as a positive "unavailable".

Use `credda.Float(72)` and `credda.String("Established")` to build these pointers in
your own fixtures and tests.

`ScoreExplainFactor.Value` and `.Contribution` are the one remaining pair that
stay `float64` even though the API can send `null` for them. Branch on
`ScoreExplainFactor.Available` before reading either; they are held for the same
version decision that governs `@credda/js`.

### Ingest an event (idempotently)

```go
res, err := client.ReportEvent(ctx, credda.ReportEventInput{
    UserID:      "user-42",
    EventType:   credda.EventContractFulfilled,
    StakeLevel:  credda.StakeHigh,
    IsVerified:  credda.Bool(true),
    CompletedAt: time.Now().UTC().Format(time.RFC3339),
}, "order-42-fulfilled") // stable Idempotency-Key → retries are exactly-once
```

Batch up to 100 at a time with `ReportEvents`; the result reports each item's
outcome (`created` / `duplicate` / `failed`) independently.

### Verify an inbound webhook

Verify against the **raw** request body, before unmarshalling.

```go
func handler(w http.ResponseWriter, r *http.Request) {
    raw, _ := io.ReadAll(r.Body)

    event, err := credda.ConstructWebhookEvent(credda.VerifyWebhookInput{
        Secret:          os.Getenv("CREDDA_WEBHOOK_SECRET"),
        RawBody:         string(raw),
        SignatureHeader: r.Header.Get("X-Credda-Signature"),
        TimestampHeader: r.Header.Get("X-Credda-Timestamp"),
    })
    if err != nil {
        http.Error(w, "invalid signature", http.StatusBadRequest)
        return
    }

    switch event.Type {
    case credda.WebhookScoreUpdated, credda.WebhookScoreBandChanged:
        data, _ := event.ScoreData()
        log.Println(data.User.ExternalID, data.Score, data.Band)
    case credda.WebhookDisputeResolved:
        data, _ := event.DisputeData()
        log.Println(data.DisputeID, data.Outcome)
    }

    w.WriteHeader(http.StatusOK)
}
```

`VerifyWebhookSignature` is the lower-level form: it returns `nil` on success or
one of the `Err*` sentinels (`ErrSignatureMismatch`, `ErrTimestampSkew`,
`ErrMissingSignature`, …) which you can branch on with `errors.Is`. The signature
is HMAC-SHA256 over `{timestamp}.{rawBody}`, hex-encoded, compared in constant
time, identical to the TypeScript SDK.

Replay tolerance defaults to 5 minutes. Override it with
`Tolerance: credda.Duration(30 * time.Second)`, or disable the freshness check
entirely with `Tolerance: credda.Duration(0)`.

## Cursor pagination

`GetScoreHistory`, `GetTimeline` and `GetWebhookDeliveries` are cursor-paginated.
Page until `NextCursor` is `nil`:

```go
var cursor string
for {
    page, err := client.GetScoreHistory(ctx, userID, &credda.ScoreHistoryQuery{
        Limit:  credda.Int(50),
        Cursor: cursor,
    })
    if err != nil {
        return err
    }
    process(page.Data)
    if page.NextCursor == nil {
        break
    }
    cursor = *page.NextCursor
}
```

## Configuration

```go
client := credda.NewClient(
    credda.WithAPIKey("crd_live_…"),
    credda.WithBaseURL("https://api.credda.io"), // default
    credda.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
)
```

Every method takes a `context.Context` first and returns `(T, error)`. Non-2xx
responses come back as `*credda.APIError` carrying `StatusCode`, `Message` (the
API's own `error`/`message` field) and `Path`.

Optional scalar fields use pointers so "absent" and "zero" stay distinguishable.
The helpers `credda.Int`, `credda.Bool`, `credda.Float`, `credda.String` and
`credda.Duration` build them inline.

### Retries

Off by default. `WithRetries(n)` enables `n` re-attempts of TRANSIENT failures
(network errors, 429, 502, 503, 504, and anything the API's error catalog marks
`retryable`):

```go
client := credda.NewClient(
    credda.WithAPIKey("crd_live_…"),
    credda.WithRetries(2),
    credda.WithRetryBackoff(300*time.Millisecond, 5*time.Second), // defaults
)
```

- GETs retry. POSTs retry **only** when you passed an idempotency key, so
  enabling this can never double-report an event. Nothing else retries, and the
  single-use `RespondToConfirmation` / `RespondToReference` endpoints never do:
  a repeat there can only turn a slow success into a confusing 409.
- A non-transient status (400, 401, 404, 409, …) returns immediately. The
  `retryable` field of the error envelope is honored, so a 500 the API marks
  retryable is re-attempted where `@credda/js` gives up.
- Backoff doubles from the base, except when the server sent `Retry-After`,
  which wins because it knows when the window resets. Both are capped by the
  second argument: a monthly-quota 429 can ask for days, and an uncapped wait
  would hang the call.
- Waiting respects the context: a cancelled or expired `ctx` returns its error
  instead of sleeping out the backoff.

## API surface

| Method | Endpoint | Key |
| --- | --- | --- |
| `ResolveToken` | `GET /api/v1/verify/:token` | none |
| `GetTrustExport` | `GET /api/v1/verify/:token/export` | none |
| `GetPublicProfessionalRecord` | `GET /api/v1/verify/:token?scope=full&professional=1` | none |
| `GetPublicReliabilityReport` | `GET /api/v1/verify/:token/reliability-report` | none |
| `GetDIDDocument` | `GET /.well-known/did.json` | none |
| `GetTrustRegistry` | `GET /.well-known/credda-trust-registry.json` | none |
| `GetScore` | `GET /api/v1/users/:id/score` | ✓ |
| `GetScores` | `POST /api/v1/users/scores` | ✓ |
| `GetScoreExplain` | `GET /api/v1/users/:id/score/explain` | ✓ |
| `GetScoreDelta` | `GET /api/v1/users/:id/score/delta` | ✓ |
| `GetScoreComponents` | `GET /api/v1/users/:id/score/components` | ✓ |
| `GetScoreHistory` | `GET /api/v1/users/:id/score/history` | ✓ |
| `GetTimeline` | `GET /api/v1/users/:id/timeline` | ✓ |
| `GetPlatforms` | `GET /api/v1/users/:id/platforms` | ✓ |
| `GetRisk` | `GET /api/v1/users/:id/risk` | ✓ |
| `GetUsage` | `GET /api/v1/usage` | ✓ |
| `ProjectScore` | `POST /api/v1/users/:id/score/project` | ✓ |
| `ReportEvent` | `POST /api/v1/events` | ✓ |
| `ReportEvents` | `POST /api/v1/events/batch` | ✓ |
| `MintShareToken` | `POST /api/v1/users/:id/share-token` | ✓ |
| `RevokeShareToken` | `DELETE /api/v1/users/:id/share-token` | ✓ |
| `ResolveDispute` | `PATCH /api/v1/disputes/:id/resolve` | ✓ |
| `CreateWebhook` | `POST /api/v1/webhooks` | ✓ |
| `ListWebhooks` | `GET /api/v1/webhooks` | ✓ |
| `UpdateWebhook` | `PATCH /api/v1/webhooks/:id` | ✓ |
| `DeleteWebhook` | `DELETE /api/v1/webhooks/:id` | ✓ |
| `TestWebhook` | `POST /api/v1/webhooks/:id/test` | ✓ |
| `GetWebhookDeliveries` | `GET /api/v1/webhooks/:id/deliveries` | ✓ |
| `GetRecentWebhookEvents` | `GET /api/v1/webhooks/deliveries` | ✓ |
| `ReplayWebhookDelivery` | `POST /api/v1/webhooks/:id/deliveries/:deliveryId/replay` | ✓ |
| `GetBenchmarks` | `GET /api/v1/benchmarks` | none |
| `GetBenchmarkDistribution` | `GET /api/v1/benchmarks/distribution` | ✓ |
| `GetUserBenchmark` | `GET /api/v1/users/:id/benchmark` | ✓ |
| `ListUsers` | `GET /api/v1/users` | ✓ |
| `GetBookSummary` | `GET /api/v1/users/summary` | ✓ |
| `GetTrustSummary` | `GET /api/v1/users/:id/trust-summary` | ✓ |
| `GetVerifiedProfile` | `GET /api/v1/users/:id/verified-profile` | ✓ |
| `RecordQualification` | `POST /api/v1/users/:id/qualifications` | ✓ |
| `GetProfessionalRecord` | `GET /api/v1/users/:id/professional-record` | ✓ |
| `MintProfessionalRecordCredential` | `POST /api/v1/users/:id/professional-record/credential` | ✓ |
| `GetReliabilityReport` | `GET /api/v1/users/:id/reliability-report` | ✓ |
| `GetDispatchReliability` | `GET /api/v1/users/:id/reliability?context=dispatch` | ✓ |
| `GetUsageMeters` | `GET /api/v1/usage/meters` | ✓ |
| `GetEventAnalytics` | `GET /api/v1/analytics/events` | ✓ |
| `GetScoreAnalytics` | `GET /api/v1/analytics/scores` | ✓ |
| `CreateActivationCampaign` | `POST /api/v1/activation/campaigns` | ✓ |
| `GetActivationCampaign` | `GET /api/v1/activation/campaigns/:id` | ✓ |
| `GetCareerExport` | `GET /api/v1/users/:id/career-export` | ✓ |
| `GetPublicCareerExport` | `GET /api/v1/verify/:token/career-export` | none |
| `GetOutcomeTemplates` | `GET /api/v1/outcome-templates` | none |
| `GetChangelog` | `GET /api/v1/changelog` | none |
| `GetOpenBadgeAchievements` | `GET /api/v1/open-badges/achievements` | none |
| `GetOpenBadgeAchievement` | `GET /api/v1/open-badges/achievements/:badgeId` | none |
| `GetCredentialIssuerMetadata` | `GET /.well-known/openid-credential-issuer` | none |
| `CreateCredentialOffer` | `POST /api/v1/users/:id/credential-offer` | ✓ |
| `CreateConfirmationRequest` | `POST /api/v1/confirmations` | ✓ |
| `CreateConfirmationBatch` | `POST /api/v1/confirmations/batch` | ✓ |
| `ListConfirmations` | `GET /api/v1/confirmations` | ✓ |
| `GetConfirmation` | `GET /api/v1/confirmations/:id` | ✓ |
| `CancelConfirmation` | `POST /api/v1/confirmations/:id/cancel` | ✓ |
| `PreviewConfirmation` | `GET /api/v1/confirmations/:id/preview` | none |
| `RespondToConfirmation` | `POST /api/v1/confirmations/:id/respond` | none |
| `CreateReferenceRequest` | `POST /api/v1/references` | ✓ |
| `ListReferences` | `GET /api/v1/references` | ✓ |
| `GetReference` | `GET /api/v1/references/:id` | ✓ |
| `CancelReference` | `POST /api/v1/references/:id/cancel` | ✓ |
| `PreviewReference` | `GET /api/v1/references/:id/preview` | none |
| `RespondToReference` | `POST /api/v1/references/:id/respond` | none |
| `CreatePolicy` | `POST /api/v1/policies` | ✓ |
| `ListPolicies` | `GET /api/v1/policies` | ✓ |
| `GetPolicy` | `GET /api/v1/policies/:id` | ✓ |
| `UpdatePolicy` | `PATCH /api/v1/policies/:id` | ✓ |
| `DeletePolicy` | `DELETE /api/v1/policies/:id` | ✓ |

Plus the catalog reads (`GetPlans`, `GetWebhookEvents`, `GetErrorCatalog`,
`GetEnums`, `GetReasonCodes`), Verified Earnings (`GetEarnings`,
`GetEarningsSummary`, `MintEarningsCredential`), agents (`RegisterAgent`,
`GetAgent`, `GetDeliveryReceipts`), monitors, screenings, ingest/imports,
`GetActivity`, `ExportEvents` and `GetUsageRange`, and the network-free helpers
`VerifyWebhookSignature`, `ConstructWebhookEvent` and `SignWebhookPayload`
(test fixtures).

### Earning `isVerified` with a counterparty confirmation

`ReportEvent` lets you *assert* `isVerified`. A confirmation request is the strong
form: propose the outcome, hand the one-time token to the named counterparty over
your own channel, and the event is written (verified) only when that distinct
party confirms. Creating the request writes no event and touches no score.

```go
created, err := c.CreateConfirmationRequest(ctx, credda.CreateConfirmationInput{
    UserID:           "user_123",
    EventType:        "CONTRACT_FULFILLED",
    CounterpartyRef:  "client@example.com",
    CounterpartyName: "Acme Studio",
    ExpiresInDays:    credda.Int(14),
}, "job-9182:confirm")
if err != nil {
    return err
}

// created.ConfirmationToken is shown ONCE. Deliver it yourself.
// The counterparty side needs no API key; the token is the capability:
res, err := c.RespondToConfirmation(ctx, created.Confirmation.ID, created.ConfirmationToken, "confirm")
```

### Benchmarks

```go
me, err := c.GetUserBenchmark(ctx, "user_123", "tenureBand")
if err != nil {
    return err
}
if me.Available {
    fmt.Println(me.Percentile, me.Comparison)
} else {
    fmt.Println("suppressed:", me.Reason) // insufficient_data | no_score
}
```

A cohort below the k-anonymity floor comes back `Available: false` with no
numbers. A percentile is a distribution fact, never a verdict.

## Deliberately not ported from `@credda/js`

Every **network** method in the TypeScript SDK exists here under the same name in
Go casing. That consistency is the point, and a parity test asserts each one's
method, path, query, body and auth match what `@credda/js` sends.

The one deliberate omission is **offline credential verification**:
`verifyTrustCredential`, `verifyVerifiableCredential`, `isCredentialRevoked` and
`verifyTrustExport` (EdDSA JWT signature checks against the issuer's JWKS/DID
document, plus StatusList2021 revocation). This module is **stdlib-only** on
purpose, and a faithful port would pull in a JOSE/JWK dependency. It is an
omission with a workaround, not a missing capability: every method that returns a
credential (`GetTrustExport`, `MintEarningsCredential`,
`MintProfessionalRecordCredential`, …) hands back the signed VC-JWT verbatim, so
verify it with the JOSE library of your choice against the JWKS in
`GetDIDDocument()`, and check revocation against the `StatusList` URL on the
payload. Webhook signature verification **is** included
(`VerifyWebhookSignature`: HMAC-SHA256 from `crypto/hmac`, no dependency needed)
and is wire-compatible with the TypeScript and Python SDKs.

The TypeScript React bindings (`CreddaProvider`, `useScore`, `useTrustToken`)
have no Go equivalent by design.

Retry semantics are ported in full: same transient status set, same
GET-and-keyed-POST-only rule, same `Retry-After`-wins-then-capped backoff, same
off-by-default. The names differ only in casing and units, `retries` /
`retryBaseMs` / `maxRetryDelayMs` becoming `WithRetries` and `WithRetryBackoff`
taking `time.Duration`. Go additionally honours the envelope's `retryable`
field, which the TypeScript client ignores.

## Tests

```sh
go vet ./...
go test ./...
```

Tests are hermetic: every HTTP interaction runs against `httptest.NewServer`.

## License

MIT © Credda. See [LICENSE](LICENSE).

---

Part of the Credda SDK family:
[`@credda/js`](https://github.com/Credda-io/credda-js) ·
[`credda-go`](https://github.com/Credda-io/credda-go) ·
[`@credda/cli`](https://github.com/Credda-io/credda-cli) ·
[`@credda/mcp-server`](https://github.com/Credda-io/credda-mcp)
