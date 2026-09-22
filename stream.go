package credda

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Server-Sent Events, because the engine serves SSE and not WebSockets: an
// event timeline is strictly one-directional server to client, and SSE
// traverses proxies and corporate middleboxes as ordinary chunked HTTP.
//
// The framing is minimal and fixed (apps/api/src/routes/stream.ts): each frame
// is an `id:` carrying the event's sequence, an `event:` carrying its type, and
// a `data:` carrying the serialized event as one line of JSON. Heartbeats
// arrive as `: heartbeat` comment lines every 15 seconds and are skipped here.
// Debug-severity events are never sent at all.

// ErrStreamRevoked is returned by StreamEvent.Err when the engine closed the
// stream because the API key that opened it was revoked.
//
// This is not a transport failure and reconnecting will not help: the gate
// refuses a new request with the same key. Authentication on a stream is
// re-checked about once a second, so a revoked key stops receiving within a
// second rather than continuing for as long as there is anything worth leaking.
var ErrStreamRevoked = errors.New("credda: the API key for this stream was revoked")

// ErrStreamIdle is returned by StreamEvent.Err when the engine dropped the
// stream after five minutes carrying nothing.
//
// The run has NOT finished — a finished run says so with a complete frame,
// which arrives as CompletedState. This is the engine declining to hold a
// connection for a run that has gone quiet, and resuming from the last
// Sequence read is the answer to it.
var ErrStreamIdle = errors.New("credda: the stream carried no event for five minutes and was dropped")

// StreamOptions configures a Stream call.
type StreamOptions struct {
	// Since is where to resume from: the sequence of the last event already
	// seen. 0 replays from the beginning. It is sent both as the `since` query
	// parameter and as the Last-Event-ID header, which is what the engine
	// prefers when both are present.
	Since int
}

// StreamEvent is one item from a stream. Exactly one of Event, ValidationEvent
// or Err is set.
type StreamEvent struct {
	// Sequence is the frame's SSE id, the event's monotonic cursor. Keep the
	// last one you saw: it is what to pass as StreamOptions.Since to resume.
	Sequence int
	// Type is the frame's SSE event name, the same value as Event.Type.
	Type string
	// Event is set by StreamInvestigation.
	Event *Event
	// ValidationEvent is set by StreamValidation.
	ValidationEvent *ValidationEvent
	// CompletedState is the terminal state the run reached, set on the final
	// item of a stream the engine closed because the run ended. Type is
	// "complete" there and neither Event nor ValidationEvent is set: this is
	// the run's own answer, not an entry in its timeline. Reopening the stream
	// would replay the backlog and the same frame.
	CompletedState string
	// Err ends the stream. io.EOF is not reported: a clean close simply closes
	// the channel. ErrStreamRevoked means the credential was withdrawn;
	// ErrStreamIdle means the run went quiet and the engine dropped the
	// connection without it having finished.
	Err error
}

// StreamInvestigation opens the investigation's live event stream.
//
// GET /api/investigations/{id}/stream
//
// The returned channel is closed when the stream ends. A stream ends when the
// run reaches a terminal state and the engine says so — the last item then
// carries CompletedState — when the caller cancels ctx, when the engine drops
// it after five minutes carrying nothing (heartbeats deliberately do not count
// as activity, and that item carries ErrStreamIdle), when the key is revoked,
// or on a transport failure. The last item before a non-clean close carries
// Err.
//
// The engine holds a process-wide budget of 64 concurrent streams and answers
// 503 TOO_MANY_STREAMS beyond it; that arrives as an error from this call
// rather than on the channel.
//
// ALWAYS cancel ctx when you stop reading. A stream you abandon holds a
// connection and two timers on the engine until its idle timeout fires.
//
// Use WithHTTPClient to raise or remove the client timeout: the default 30s
// applies to the whole response and will cut a healthy stream.
func (c *Client) StreamInvestigation(ctx context.Context, id string, opts *StreamOptions) (<-chan StreamEvent, error) {
	return stream(ctx, c, "/investigations/"+esc(id)+"/stream", opts, func(raw []byte, out *StreamEvent) error {
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		out.Event = &e
		return nil
	})
}

// StreamValidation opens the validation's live event stream.
//
// GET /api/validations/{id}/stream
//
// Same semantics as StreamInvestigation; the items carry ValidationEvent.
func (c *Client) StreamValidation(ctx context.Context, id string, opts *StreamOptions) (<-chan StreamEvent, error) {
	return stream(ctx, c, "/validations/"+esc(id)+"/stream", opts, func(raw []byte, out *StreamEvent) error {
		var e ValidationEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			return err
		}
		out.ValidationEvent = &e
		return nil
	})
}

func stream(
	ctx context.Context,
	c *Client,
	path string,
	opts *StreamOptions,
	decode func([]byte, *StreamEvent) error,
) (<-chan StreamEvent, error) {
	since := 0
	if opts != nil && opts.Since > 0 {
		since = opts.Since
	}
	qs := url.Values{}
	qs.Set("since", strconv.Itoa(since))

	ro := requestOptions{
		method: http.MethodGet,
		path:   withQuery(path, qs),
		accept: "text/event-stream",
		raw:    true,
		headers: map[string]string{
			"Last-Event-ID": strconv.Itoa(since),
			"Cache-Control": "no-cache",
		},
	}
	// Retries are off for a stream even when configured: doRaw's retry applies
	// to a request that either succeeds or fails, and a stream that dropped
	// mid-flight has already delivered events the caller has seen. Resuming is
	// the caller's decision, from the last Sequence they read.
	resp, err := c.attempt(ctx, ro, nil, nil)
	if err != nil {
		return nil, err
	}

	out := make(chan StreamEvent)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		readFrames(ctx, resp.Body, decode, out)
	}()
	return out, nil
}

// readFrames parses the SSE byte stream into StreamEvents.
//
// A frame is terminated by a blank line. `id:`, `event:` and `data:` are the
// only fields the engine emits; anything else, and any `:` comment line
// (the heartbeat), is skipped. A frame with no data is not an event.
func readFrames(ctx context.Context, body interface{ Read([]byte) (int, error) },
	decode func([]byte, *StreamEvent) error, out chan<- StreamEvent) {

	scanner := bufio.NewScanner(body)
	// Event payloads carry stack traces and command output; the default 64KB
	// token limit is not enough for one line of that JSON.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		name string
		id   string
		data strings.Builder
	)

	emit := func() bool {
		defer func() {
			name, id = "", ""
			data.Reset()
		}()
		if data.Len() == 0 {
			return true
		}
		// The three notices the engine says before it closes, so a consumer
		// learns why the connection ended rather than reading a silent drop as
		// a dropped link. None of them is an event: they carry no `id:` and
		// their payload is not an event row.
		//
		// Each one ENDS the stream, as the StreamEvent doc states: after it we
		// stop reading and let the deferred Body.Close run, rather than scanning
		// on. The engine closes the connection right after any of them, so today
		// the next Scan would return EOF regardless; returning false here makes
		// "Err ends the stream" a fact about this reader and not a bet on the
		// server never emitting a frame after a terminal notice.
		switch name {
		case "unauthenticated":
			send(ctx, out, StreamEvent{Err: ErrStreamRevoked})
			return false
		case "idle":
			send(ctx, out, StreamEvent{Err: ErrStreamIdle})
			return false
		case "complete":
			var payload struct {
				State string `json:"state"`
			}
			// A malformed payload still ends the stream: the close is the fact,
			// and the state is what it carried.
			_ = json.Unmarshal([]byte(data.String()), &payload)
			send(ctx, out, StreamEvent{Type: name, CompletedState: payload.State})
			return false
		}
		ev := StreamEvent{Type: name}
		if n, err := strconv.Atoi(id); err == nil {
			ev.Sequence = n
		}
		if err := decode([]byte(data.String()), &ev); err != nil {
			return send(ctx, out, StreamEvent{Err: fmt.Errorf("credda: decoding stream event %q: %w", name, err)})
		}
		return send(ctx, out, ev)
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if !emit() {
				return
			}
		case strings.HasPrefix(line, ":"):
			// Comment. The heartbeat lands here.
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "id:"):
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		}
	}
	// A final frame with no trailing blank line.
	if !emit() {
		return
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		send(ctx, out, StreamEvent{Err: fmt.Errorf("credda: reading event stream: %w", err)})
	}
}

// send reports whether the caller is still reading.
func send(ctx context.Context, out chan<- StreamEvent, ev StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
