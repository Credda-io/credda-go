package credda

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// The create route's idempotency key, and the two things about it this client
// must not let a caller lose: that a retried create carries one, and that a key
// stays bound to the body it was minted for.

const createdBody = `{"investigation":{"id":"inv_1","state":"CREATED"},` +
	`"hypotheses":[],"patches":[],"verifications":[],"evidenceCount":0,"latestSequence":0}`

func TestCreateInvestigationOnceSendsTheKeyAsAHeaderAndNotOnTheBody(t *testing.T) {
	c, got := newTestServer(t, 201, createdBody)
	req, err := IdempotentCreateWithKey("claim-42", CreateInvestigationInput{
		RepositoryID: "repo_1", IssueTitle: "t", IssueBody: "b",
	})
	if err != nil {
		t.Fatalf("IdempotentCreateWithKey: %v", err)
	}
	if _, err := c.CreateInvestigationOnce(context.Background(), req); err != nil {
		t.Fatalf("CreateInvestigationOnce: %v", err)
	}
	if got.Idempotency != "claim-42" {
		t.Errorf("Idempotency-Key = %q, want claim-42", got.Idempotency)
	}
	// createBody is .strict(), so a key on the body would be a 400 naming it.
	if strings.Contains(got.Body, "claim-42") || strings.Contains(got.Body, "dempotency") {
		t.Errorf("the key travelled on the body: %s", got.Body)
	}
}

// The status line is the only thing that separates a run this call opened from
// one the engine already had: the two bodies are identical.
func TestCreationStatusIsReadOffTheStatusLine(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   CreationStatus
		opened bool
	}{
		{http.StatusCreated, StatusCreated, true},
		{http.StatusOK, StatusReplayed, false},
	} {
		c, _ := newTestServer(t, tc.status, createdBody)
		req, err := NewIdempotentCreate(CreateInvestigationInput{RepositoryID: "repo_1", IssueTitle: "t", IssueBody: "b"})
		if err != nil {
			t.Fatalf("NewIdempotentCreate: %v", err)
		}
		out, err := c.CreateInvestigationOnce(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateInvestigationOnce: %v", err)
		}
		if out.Status != tc.want {
			t.Errorf("HTTP %d: Status = %q, want %q", tc.status, out.Status, tc.want)
		}
		if out.Opened() != tc.opened {
			t.Errorf("HTTP %d: Opened() = %v, want %v", tc.status, out.Opened(), tc.opened)
		}
		if out.Detail == nil || out.Detail.Investigation.State != "CREATED" {
			t.Errorf("HTTP %d: the run did not decode", tc.status)
		}
	}
}

// A key is minted per body. Rewriting the report means a new key, so the 409 the
// engine answers for a key over a different body is not reachable by editing a
// struct field: the pair is unexported and made together.
func TestEachIdempotentCreateCarriesItsOwnKey(t *testing.T) {
	first, err := NewIdempotentCreate(CreateInvestigationInput{RepositoryID: "repo_1", IssueTitle: "one", IssueBody: "b"})
	if err != nil {
		t.Fatalf("NewIdempotentCreate: %v", err)
	}
	second, err := NewIdempotentCreate(CreateInvestigationInput{RepositoryID: "repo_1", IssueTitle: "two", IssueBody: "b"})
	if err != nil {
		t.Fatalf("NewIdempotentCreate: %v", err)
	}
	if first.Key() == second.Key() {
		t.Fatal("two reports were minted the same key; the second would be refused as a reuse")
	}
	if first.Input().IssueTitle != "one" {
		t.Errorf("Input() = %q, want the body the key was minted for", first.Input().IssueTitle)
	}
}

// The zero value carries no key, so sending it would be a create with no header
// and a retry policy that thinks it has one.
func TestZeroIdempotentCreateIsRefusedWithoutASpentRequest(t *testing.T) {
	c, got := newTestServer(t, 201, createdBody)
	_, err := c.CreateInvestigationOnce(context.Background(), IdempotentCreate{})
	if !errors.Is(err, ErrInvalidIdempotencyKey) {
		t.Fatalf("err = %v, want ErrInvalidIdempotencyKey", err)
	}
	if got.Calls != 0 {
		t.Errorf("the request was sent %d times; a keyless idempotent create must not reach the engine", got.Calls)
	}
}

func TestIdempotencyKeysAreValidatedAgainstTheEnginesCeiling(t *testing.T) {
	if _, err := ParseIdempotencyKey(""); !errors.Is(err, ErrInvalidIdempotencyKey) {
		t.Errorf("an empty key was accepted")
	}
	if _, err := ParseIdempotencyKey(strings.Repeat("k", 256)); !errors.Is(err, ErrInvalidIdempotencyKey) {
		t.Errorf("a 256-character key was accepted; the engine's ceiling is 255")
	}
	if _, err := ParseIdempotencyKey(strings.Repeat("k", 255)); err != nil {
		t.Errorf("a 255-character key was refused: %v", err)
	}
	a, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey: %v", err)
	}
	b, err := NewIdempotencyKey()
	if err != nil {
		t.Fatalf("NewIdempotencyKey: %v", err)
	}
	if a == b {
		t.Fatal("two minted keys collided")
	}
}

// safeToRepeat is the one place "retried" is decided, and the key is its whole
// condition for a non-GET. A POST without one is not repeatable and there is no
// flag that makes it so.
func TestOnlyAKeyMakesAWriteRepeatable(t *testing.T) {
	if (requestOptions{method: http.MethodPost}).safeToRepeat() {
		t.Error("a keyless POST is repeatable; a repeated create opens a second run")
	}
	if !(requestOptions{method: http.MethodPost, idempotencyKey: "k"}).safeToRepeat() {
		t.Error("a keyed POST is not repeatable; the key exists so that it is")
	}
	if !(requestOptions{method: http.MethodGet}).safeToRepeat() {
		t.Error("a GET is not repeatable")
	}
}
