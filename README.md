# `credda` — official Go SDK for the Credda API

A dependency-free Go client for the [Credda](https://credda.io) Reliability Score
API. It mirrors the TypeScript SDK (`@credda/js`) endpoint for endpoint: public
share-token reads, platform-key scored reads, event ingestion, share tokens,
dispute resolution, webhook management, and inbound webhook verification.

Standard library only — `net/http`, `crypto/hmac`, `encoding/json`. No third-party
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

trust, err := client.ResolveToken(ctx, "tok_abc123")
if err != nil {
    log.Fatal(err)
}
fmt.Println(trust.FinalScore, trust.ScoreBand)
```

`ResolveToken`, `GetTrustExport`, `GetDIDDocument` and `GetTrustRegistry` are the
only methods that work without a key — they are safe for untrusted contexts.

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
> browser bundle — use a share token there.

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
time — identical to the TypeScript SDK.

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

## API surface

| Method | Endpoint | Key |
| --- | --- | --- |
| `ResolveToken` | `GET /api/v1/verify/:token` | — |
| `GetTrustExport` | `GET /api/v1/verify/:token/export` | — |
| `GetPublicProfessionalRecord` | `GET /api/v1/verify/:token?scope=full&professional=1` | — |
| `GetPublicReliabilityReport` | `GET /api/v1/verify/:token/reliability-report` | — |
| `GetDIDDocument` | `GET /.well-known/did.json` | — |
| `GetTrustRegistry` | `GET /.well-known/credda-trust-registry.json` | — |
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
| `GetBenchmarks` | `GET /api/v1/benchmarks` | — |
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
| `GetPublicCareerExport` | `GET /api/v1/verify/:token/career-export` | — |
| `GetOutcomeTemplates` | `GET /api/v1/outcome-templates` | — |
| `GetChangelog` | `GET /api/v1/changelog` | — |
| `GetOpenBadgeAchievements` | `GET /api/v1/open-badges/achievements` | — |
| `GetOpenBadgeAchievement` | `GET /api/v1/open-badges/achievements/:badgeId` | — |
| `GetCredentialIssuerMetadata` | `GET /.well-known/openid-credential-issuer` | — |
| `CreateCredentialOffer` | `POST /api/v1/users/:id/credential-offer` | ✓ |
| `CreateConfirmationRequest` | `POST /api/v1/confirmations` | ✓ |
| `CreateConfirmationBatch` | `POST /api/v1/confirmations/batch` | ✓ |
| `ListConfirmations` | `GET /api/v1/confirmations` | ✓ |
| `GetConfirmation` | `GET /api/v1/confirmations/:id` | ✓ |
| `CancelConfirmation` | `POST /api/v1/confirmations/:id/cancel` | ✓ |
| `PreviewConfirmation` | `GET /api/v1/confirmations/:id/preview` | — |
| `RespondToConfirmation` | `POST /api/v1/confirmations/:id/respond` | — |
| `CreateReferenceRequest` | `POST /api/v1/references` | ✓ |
| `ListReferences` | `GET /api/v1/references` | ✓ |
| `GetReference` | `GET /api/v1/references/:id` | ✓ |
| `CancelReference` | `POST /api/v1/references/:id/cancel` | ✓ |
| `PreviewReference` | `GET /api/v1/references/:id/preview` | — |
| `RespondToReference` | `POST /api/v1/references/:id/respond` | — |
| `CreatePolicy` | `POST /api/v1/policies` | ✓ |
| `ListPolicies` | `GET /api/v1/policies` | ✓ |
| `GetPolicy` | `GET /api/v1/policies/:id` | ✓ |
| `UpdatePolicy` | `PATCH /api/v1/policies/:id` | ✓ |
| `DeletePolicy` | `DELETE /api/v1/policies/:id` | ✓ |

Plus the catalog reads (`GetPlans`, `GetWebhookEvents`, `GetErrorCatalog`,
`GetEnums`, `GetReasonCodes`), Verified Earnings (`GetEarnings`,
`GetEarningsSummary`, `MintEarningsCredential`), agents (`RegisterAgent`,
`GetAgent`, `GetDeliveryReceipts`), monitors, screenings, ingest/imports,
`GetActivity`, `ExportEvents` and `GetUsageRange` — and the network-free helpers
`VerifyWebhookSignature`, `ConstructWebhookEvent` and `SignWebhookPayload`
(test fixtures).

### Earning `isVerified` with a counterparty confirmation

`ReportEvent` lets you *assert* `isVerified`. A confirmation request is the strong
form: propose the outcome, hand the one-time token to the named counterparty over
your own channel, and the event is written — verified — only when that distinct
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

// created.ConfirmationToken is shown ONCE — deliver it yourself.
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
Go casing — that consistency is the point, and a parity test asserts each one's
method, path, query, body and auth match what `@credda/js` sends.

The one deliberate omission is **offline credential verification** —
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
(`VerifyWebhookSignature` — HMAC-SHA256 from `crypto/hmac`, no dependency needed)
and is wire-compatible with the TypeScript and Python SDKs.

The TypeScript React bindings (`CreddaProvider`, `useScore`, `useTrustToken`)
have no Go equivalent by design.

## Tests

```sh
go vet ./...
go test ./...
```

Tests are hermetic — every HTTP interaction runs against `httptest.NewServer`.
