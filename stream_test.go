package credda

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sseServer writes the given frames verbatim, flushing after each, then closes.
func sseServer(t *testing.T, frames ...string) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Method = r.Method
		got.Path = r.URL.Path
		got.Query = r.URL.RawQuery
		got.Accept = r.Header.Get("Accept")
		got.Body = r.Header.Get("Last-Event-ID")
		got.Calls++

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for _, f := range frames {
			_, _ = w.Write([]byte(f))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient(WithBaseURL(srv.URL), WithAPIKey("k")), got
}

func TestStreamInvestigationDecodesFrames(t *testing.T) {
	c, got := sseServer(t,
		"id: 1\nevent: state.changed\ndata: "+
			`{"id":"e1","investigationId":"inv_1","sequence":1,"type":"state.changed",`+
			`"severity":"info","summary":"CREATED -> PREPARING_ENVIRONMENT",`+
			`"state":"PREPARING_ENVIRONMENT","agentRunId":null,"toolCallId":null,`+
			`"evidenceIds":[],"metadata":{},"createdAt":"2026-08-27T10:00:00.000Z"}`+"\n\n",
		": heartbeat\n\n",
		"id: 2\nevent: evidence.recorded\ndata: "+
			`{"id":"e2","investigationId":"inv_1","sequence":2,"type":"evidence.recorded",`+
			`"severity":"info","summary":"captured a failure signature","state":null,`+
			`"agentRunId":"run_1","toolCallId":"tc_9","evidenceIds":["ev_1"],`+
			`"metadata":{"type":"REPRODUCTION"},"createdAt":"2026-08-27T10:02:00.000Z"}`+"\n\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", &StreamOptions{Since: 0})
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}

	var seen []StreamEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		seen = append(seen, ev)
	}

	if got.Path != "/api/investigations/inv_1/stream" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query != "since=0" {
		t.Errorf("query = %q, want since=0", got.Query)
	}
	if got.Accept != "text/event-stream" {
		t.Errorf("Accept = %q", got.Accept)
	}
	if len(seen) != 2 {
		t.Fatalf("events = %d, want 2 (the heartbeat is not an event)", len(seen))
	}
	if seen[0].Sequence != 1 || seen[0].Type != "state.changed" {
		t.Errorf("frame 0 = %+v", seen[0])
	}
	if seen[0].Event == nil || seen[0].Event.State == nil || *seen[0].Event.State != "PREPARING_ENVIRONMENT" {
		t.Errorf("frame 0 payload = %+v", seen[0].Event)
	}
	if seen[1].Sequence != 2 || seen[1].Event == nil {
		t.Fatalf("frame 1 = %+v", seen[1])
	}
	if len(seen[1].Event.EvidenceIDs) != 1 || seen[1].Event.EvidenceIDs[0] != "ev_1" {
		t.Errorf("evidence ids = %v", seen[1].Event.EvidenceIDs)
	}
	// An event that is not a state transition has no state, and nil must stay
	// nil rather than becoming an empty state name.
	if seen[1].Event.State != nil {
		t.Errorf("State = %q, want nil for a non-transition event", *seen[1].Event.State)
	}
}

// Resuming. The cursor goes out as both `since` and Last-Event-ID; the engine
// prefers the header when both are present, so a client that sent only the
// query parameter would replay everything on a reconnect.
func TestStreamSendsTheCursorAsBothQueryAndHeader(t *testing.T) {
	c, got := sseServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamValidation(ctx, "val_1", &StreamOptions{Since: 412})
	if err != nil {
		t.Fatalf("StreamValidation: %v", err)
	}
	for range ch {
	}

	if got.Path != "/api/validations/val_1/stream" {
		t.Errorf("path = %q", got.Path)
	}
	if got.Query != "since=412" {
		t.Errorf("query = %q, want since=412", got.Query)
	}
	if got.Body != "412" {
		t.Errorf("Last-Event-ID = %q, want 412", got.Body)
	}
}

func TestStreamValidationDecodesValidationEvents(t *testing.T) {
	c, _ := sseServer(t,
		"id: 7\nevent: check.completed\ndata: "+
			`{"id":"ve1","validationId":"val_1","checkId":"c1","sequence":7,`+
			`"type":"check.completed","severity":"warn","summary":"checkout over 1000 FAILED",`+
			`"state":"CONFIRMING_FINDINGS","data":{"status":"FAILED","baseStatus":"PASSED"},`+
			`"createdAt":"2026-08-27T10:03:00.000Z"}`+"\n\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamValidation(ctx, "val_1", nil)
	if err != nil {
		t.Fatalf("StreamValidation: %v", err)
	}

	ev, ok := <-ch
	if !ok {
		t.Fatal("the stream closed with no event")
	}
	if ev.Err != nil {
		t.Fatalf("stream error: %v", ev.Err)
	}
	if ev.ValidationEvent == nil {
		t.Fatal("a validation stream produced no ValidationEvent")
	}
	if ev.Event != nil {
		t.Error("a validation stream also produced an investigation Event")
	}
	if ev.ValidationEvent.CheckID == nil || *ev.ValidationEvent.CheckID != "c1" {
		t.Errorf("CheckID = %v", ev.ValidationEvent.CheckID)
	}
	if len(ev.ValidationEvent.Data) == 0 {
		t.Error("the event data bag was dropped")
	}
	for range ch {
	}
}

// The engine says the credential was withdrawn before it closes, so a consumer
// learns the difference between a revoked key and a dropped connection.
// Reconnecting with the same key would just be refused 401, so this must not
// look like a transient failure.
func TestStreamSurfacesRevocation(t *testing.T) {
	c, _ := sseServer(t,
		"id: 1\nevent: state.changed\ndata: "+
			`{"id":"e1","investigationId":"inv_1","sequence":1,"type":"state.changed",`+
			`"severity":"info","summary":"running","state":null,"agentRunId":null,`+
			`"toolCallId":null,"evidenceIds":[],"metadata":{},"createdAt":"2026-08-27T10:00:00.000Z"}`+"\n\n",
		"event: unauthenticated\ndata: {\"reason\":\"the API key for this stream was revoked\"}\n\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}

	var last error
	events := 0
	for ev := range ch {
		if ev.Err != nil {
			last = ev.Err
			continue
		}
		events++
	}
	if events != 1 {
		t.Errorf("events = %d, want 1 before the revocation", events)
	}
	if !errors.Is(last, ErrStreamRevoked) {
		t.Fatalf("stream ended with %v, want ErrStreamRevoked", last)
	}
}

// The stream budget is a process-wide 64. Exceeding it is a 503 on the OPEN,
// not an item on the channel, because there is no channel to put it on.
func TestStreamBudgetRefusalIsAnErrorFromTheCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"code":"TOO_MANY_STREAMS","message":"Too many open event streams"}}`))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("k"))
	ch, err := c.StreamInvestigation(context.Background(), "inv_1", nil)
	if err == nil {
		t.Fatal("want an error for a refused stream")
	}
	if ch != nil {
		t.Error("a refused stream returned a channel")
	}
	apiErr, _ := AsAPIError(err)
	if apiErr == nil || apiErr.Code != CodeTooManyStreams {
		t.Errorf("err = %+v, want %s", apiErr, CodeTooManyStreams)
	}
}

// Cancelling ctx must stop the reader and close the channel. A stream nobody
// closes holds a connection and two timers on the engine until its idle
// timeout fires five minutes later.
func TestStreamStopsOnContextCancellation(t *testing.T) {
	// A server that keeps writing until the client goes away.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for i := 1; ; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			frame := fmt.Sprintf("id: %d\nevent: tick\ndata: "+
				`{"id":"e%d","investigationId":"inv_1","sequence":%d,"type":"tick",`+
				`"severity":"info","summary":"t","state":null,"agentRunId":null,`+
				`"toolCallId":null,"evidenceIds":[],"metadata":{},"createdAt":"2026-08-27T10:00:00.000Z"}`+
				"\n\n", i, i, i)
			if _, err := w.Write([]byte(frame)); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(time.Millisecond)
		}
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL), WithAPIKey("k"),
		WithHTTPClient(&http.Client{}))

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}

	<-ch // one event proves it is flowing
	cancel()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not stop after the context was cancelled")
	}
}

// A frame whose payload is not decodable ends the stream with a named error
// rather than a silent drop: an event a client never saw is a gap in a timeline
// it believes is complete.
func TestStreamReportsAnUndecodableFrame(t *testing.T) {
	c, _ := sseServer(t, "id: 1\nevent: broken\ndata: {not json\n\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}
	ev := <-ch
	if ev.Err == nil {
		t.Fatal("an undecodable frame was passed off as an event")
	}
	for range ch {
	}
}

// An empty stream — the investigation exists and has produced nothing since the
// cursor — closes cleanly with no items and no error.
func TestEmptyStreamClosesCleanly(t *testing.T) {
	c, _ := sseServer(t, ": heartbeat\n\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}
	for ev := range ch {
		t.Fatalf("unexpected item: %+v", ev)
	}
}

// The run's own answer. The engine drains the timeline, then says `complete`
// with the terminal state and closes; before this was handled the frame was
// decoded as if it were an event and reached the caller as an empty Event with
// sequence 0 — a phantom entry in a timeline, and no way to tell a finished run
// from a dropped link.
func TestStreamSurfacesTheTerminalState(t *testing.T) {
	c, _ := sseServer(t,
		"id: 1\nevent: state.changed\ndata: "+
			`{"id":"e1","investigationId":"inv_1","sequence":1,"type":"state.changed",`+
			`"severity":"info","summary":"running","state":null,"agentRunId":null,`+
			`"toolCallId":null,"evidenceIds":[],"metadata":{},"createdAt":"2026-08-27T10:00:00.000Z"}`+"\n\n",
		"event: complete\ndata: {\"state\":\"RESOLVED\"}\n\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}

	var items []StreamEvent
	for ev := range ch {
		items = append(items, ev)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want the event and the completion", len(items))
	}
	last := items[1]
	if last.Err != nil {
		t.Fatalf("a finished run reported an error: %v", last.Err)
	}
	if last.CompletedState != "RESOLVED" {
		t.Errorf("CompletedState = %q, want RESOLVED", last.CompletedState)
	}
	if last.Event != nil {
		t.Error("the completion frame was decoded as an event")
	}
}

// A run that went quiet without finishing. Not a transport failure and not an
// answer: the engine declines to hold the connection, and resuming from the
// last sequence is what a client does about it.
func TestStreamSurfacesTheIdleDrop(t *testing.T) {
	c, _ := sseServer(t,
		"event: idle\ndata: {\"reason\":\"no events for five minutes; the run has not reached a terminal state\"}\n\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.StreamInvestigation(ctx, "inv_1", nil)
	if err != nil {
		t.Fatalf("StreamInvestigation: %v", err)
	}
	ev := <-ch
	if !errors.Is(ev.Err, ErrStreamIdle) {
		t.Fatalf("ev = %+v, want ErrStreamIdle", ev)
	}
	if ev.Event != nil {
		t.Error("the idle frame was decoded as an event")
	}
	for range ch {
	}
}
