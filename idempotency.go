package credda

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// IdempotencyHeader is the header the engine reads on POST /api/investigations
// (IDEMPOTENCY_HEADER in apps/api/src/routes/investigations.ts). It is the only
// request header this client sends besides Authorization and Content-Type.
const IdempotencyHeader = "Idempotency-Key"

// maxIdempotencyKeyLen is the engine's ceiling, from idempotencyHeader on the
// create route. A longer key is a 400 there, so it is refused here before a
// request is spent finding that out.
const maxIdempotencyKeyLen = 255

// IdempotencyKey makes CreateInvestigationOnce safe to repeat.
//
// # What the engine promises
//
// The create route reads this key from a header and, with
// createInvestigationOnce in apps/api/src/context.ts, writes it and a hash of
// the parsed body alongside the run in one transaction:
//
//   - 201, first request under the key — the run was created.
//   - 200, the SAME body again under the key — the same run, returned again.
//     Nothing was created and nothing was billed.
//   - 409 CodeIdempotencyKeyReused, a DIFFERENT body under the key — and
//     neither run is disclosed: the earlier one would answer a question this
//     caller never asked, and a new one is the duplicate the key was sent to
//     prevent.
//   - No header at all — byte for byte the behaviour the route had before the
//     header existed. One row per request, no claim written.
//
// The claim is scoped to the organisation the bearer key names and never
// expires; it lives as long as the investigation and is deleted with it.
//
// # Why this package does not mint one for you
//
// Running an investigation spends a model budget, so a create repeated because
// a socket died is a second bill. That is what the key prevents, and it is why
// CreateInvestigationOnce is the one write WithRetries repeats.
//
// It would have been easy to attach a key to every create and retry them all.
// This package deliberately does not, and the engine is the reason. A key means
// "these requests are one intent", and the engine's handler is explicit that
// only the caller can know that: a key derived from the body would make the
// second run of the same report impossible, and rerunning one report is a real
// thing to want, because the engine is not deterministic and the obvious move
// after a run that reproduced nothing is to run it again. A key this client
// invented per call would also cover only its OWN retries — the process that
// crashes and re-sends after a restart, which is the case that actually
// double-bills, cannot name a key it never saw. So the caller mints it with
// NewIdempotencyKey, records it, and can send it again tomorrow with
// ParseIdempotencyKey.
//
// It is a named type rather than a string parameter for the reason
// CancellationStatus is one: it cannot be produced by writing a literal in the
// wrong argument position, and every value of it has been through a constructor
// that enforced the engine's ceiling.
type IdempotencyKey string

// ErrInvalidIdempotencyKey is returned for a key the engine would refuse: empty,
// or longer than 255 characters.
var ErrInvalidIdempotencyKey = errors.New("credda: invalid idempotency key")

// NewIdempotencyKey mints a fresh key from crypto/rand. Record it before you
// send it: a key you cannot read back after a restart cannot deduplicate the
// request that restart is about to repeat.
//
// It fails only when the system's entropy source does. There is no math/rand
// fallback, deliberately: two callers colliding on a key is one organisation
// being handed the other's investigation.
func NewIdempotencyKey() (IdempotencyKey, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("credda: minting an idempotency key: %w", err)
	}
	return IdempotencyKey(hex.EncodeToString(b[:])), nil
}

// ParseIdempotencyKey adopts a key the caller already holds — one read back
// from their own job record after a restart, which is the case a
// client-invented key cannot serve.
func ParseIdempotencyKey(s string) (IdempotencyKey, error) {
	if s == "" {
		return "", fmt.Errorf("%w: it must not be empty", ErrInvalidIdempotencyKey)
	}
	if len(s) > maxIdempotencyKeyLen {
		return "", fmt.Errorf("%w: at most %d characters, got %d", ErrInvalidIdempotencyKey, maxIdempotencyKeyLen, len(s))
	}
	return IdempotencyKey(s), nil
}

// IdempotentCreate is one key bound to one body — the value
// CreateInvestigationOnce takes, and the reason a key cannot drift onto a
// different report by accident.
//
// The engine's second promise is the sharp one: the same key over a DIFFERENT
// body is a 409, and a method taking the key and the body as two independent
// arguments would make that mistake reachable by editing one of them. Here they
// are made together, the fields are unexported, and the zero value is refused.
// Changing the report means calling NewIdempotentCreate again, which mints a
// new key.
//
//	req, err := credda.NewIdempotentCreate(credda.CreateInvestigationInput{
//		RepositoryID: repoID,
//		IssueTitle:   title,
//		IssueBody:    body,
//	})
//	if err != nil {
//		return err
//	}
//	if err := jobs.Record(ticketID, req.Key()); err != nil { // so a restart re-sends it
//		return err
//	}
//	created, err := client.CreateInvestigationOnce(ctx, req)
type IdempotentCreate struct {
	key   IdempotencyKey
	input CreateInvestigationInput
}

// NewIdempotentCreate pairs a report with a freshly minted key.
//
// Calling this IS the caller's statement that a repeat of this request is the
// same intent. Nothing in this package makes that statement for them.
func NewIdempotentCreate(in CreateInvestigationInput) (IdempotentCreate, error) {
	key, err := NewIdempotencyKey()
	if err != nil {
		return IdempotentCreate{}, err
	}
	return IdempotentCreate{key: key, input: in}, nil
}

// IdempotentCreateWithKey pairs a report with a key the caller already recorded
// — the call a restarted process makes to re-send the request it is not sure
// arrived. The body must be the one the key was minted for; a different body
// under the same key is a 409 CodeIdempotencyKeyReused.
func IdempotentCreateWithKey(key IdempotencyKey, in CreateInvestigationInput) (IdempotentCreate, error) {
	parsed, err := ParseIdempotencyKey(string(key))
	if err != nil {
		return IdempotentCreate{}, err
	}
	return IdempotentCreate{key: parsed, input: in}, nil
}

// Key is the key this request is claimed under. Record it: sending it again
// returns the same run rather than opening a second.
func (r IdempotentCreate) Key() IdempotencyKey { return r.key }

// Input is the body the key stands for.
func (r IdempotentCreate) Input() CreateInvestigationInput { return r.input }
