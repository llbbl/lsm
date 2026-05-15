// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordedRequest is a single request captured by fakeOTLP.
type recordedRequest struct {
	Headers http.Header
	Body    []byte
}

// fakeOTLP is a test double for an OTLP/HTTP logs endpoint. It records
// every inbound request. Status behavior is controlled by a small
// scripted queue: pop the next status from `statuses`; if empty, return
// `defaultStatus`. A "hang" status (0) blocks the handler on `release`
// until the test closes it — used to exercise queue saturation and
// close-timeout paths.
type fakeOTLP struct {
	mu            sync.Mutex
	requests      []recordedRequest
	statuses      []int
	defaultStatus int
	release       chan struct{}
	hang          bool
	notify        chan struct{} // closed-then-replaced on each request
}

func newFakeOTLP() *fakeOTLP {
	return &fakeOTLP{
		defaultStatus: 200,
		release:       make(chan struct{}),
		notify:        make(chan struct{}),
	}
}

func (f *fakeOTLP) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{
		Headers: r.Header.Clone(),
		Body:    body,
	})
	status := f.defaultStatus
	if len(f.statuses) > 0 {
		status = f.statuses[0]
		f.statuses = f.statuses[1:]
	}
	hang := f.hang
	// Wake any waiters that a request landed.
	prev := f.notify
	f.notify = make(chan struct{})
	close(prev)
	f.mu.Unlock()

	if hang {
		<-f.release
	}
	w.WriteHeader(status)
}

func (f *fakeOTLP) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeOTLP) snapshot() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// waitForRequests blocks until requestCount() >= n or the deadline
// elapses. Returns true on success.
func (f *fakeOTLP) waitForRequests(n int, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for {
		if f.requestCount() >= n {
			return true
		}
		f.mu.Lock()
		ch := f.notify
		f.mu.Unlock()
		remaining := time.Until(end)
		if remaining <= 0 {
			return false
		}
		select {
		case <-ch:
		case <-time.After(remaining):
			return f.requestCount() >= n
		}
	}
}

func startFake(t *testing.T) (*fakeOTLP, *httptest.Server) {
	t.Helper()
	f := newFakeOTLP()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(func() {
		// Make sure any hung handler can return so srv.Close finishes.
		select {
		case <-f.release:
		default:
			close(f.release)
		}
		srv.Close()
	})
	return f, srv
}

func mkTransformer() Transformer {
	return Transformer{
		Salt:     []byte("test-salt-32-bytes-long-fixed-AAAA"),
		HostName: "test-host",
	}
}

func mkEvent(seq uint64, name string) Event {
	return Event{
		Seq:       seq,
		Timestamp: time.Date(2026, 5, 14, 10, 30, 45, 0, time.UTC),
		Event:     name,
		App:       "webapp",
		Env:       "production",
		Actor: Actor{
			PPID: 1234, ParentComm: "zsh", TTY: "/dev/ttys001",
			AgentMarker: "claude-code", UID: 501,
		},
		Fields: map[string]any{"key": "DATABASE_URL"},
	}
}

func TestNewOTLPSink_RejectsBadConfig(t *testing.T) {
	cases := map[string]OTLPSinkConfig{
		"empty endpoint": {Transformer: mkTransformer()},
		"nil salt":       {Endpoint: "http://x", Transformer: Transformer{HostName: "h"}},
	}
	for name, cfg := range cases {
		if _, err := NewOTLPSink(cfg); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestOTLPSink_SingleEventFlushesViaClose(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      10,
		BatchWindow:    time.Hour, // we'll use Close to force flush
		QueueCap:       10,
		MaxRetries:     1,
		RetryBaseDelay: 5 * time.Millisecond,
		HTTPClient:     &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}

	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := f.requestCount(); got != 1 {
		t.Fatalf("requestCount = %d, want 1", got)
	}

	req := f.snapshot()[0]
	if req.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("missing Content-Type, got %q", req.Headers.Get("Content-Type"))
	}

	// Decode payload and sanity check shape.
	var payload struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					Attributes []struct {
						Key   string
						Value struct {
							StringValue string
						}
					}
					Body struct{ StringValue string }
				}
			}
		}
	}
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.ResourceLogs) != 1 {
		t.Fatalf("resourceLogs = %d, want 1", len(payload.ResourceLogs))
	}
	recs := payload.ResourceLogs[0].ScopeLogs[0].LogRecords
	if len(recs) != 1 {
		t.Fatalf("logRecords = %d, want 1", len(recs))
	}
	attrs := map[string]string{}
	for _, a := range recs[0].Attributes {
		attrs[a.Key] = a.Value.StringValue
	}
	for _, k := range []string{"event", "app", "env", "host", "tty_present", "agent_marker"} {
		if _, ok := attrs[k]; !ok {
			t.Errorf("missing attribute %q in %v", k, attrs)
		}
	}
	if attrs["event"] != "set.success" {
		t.Errorf("event attr = %q, want %q", attrs["event"], "set.success")
	}
	if attrs["host"] != "test-host" {
		t.Errorf("host attr = %q, want %q", attrs["host"], "test-host")
	}
	// Body must be a JSON string carrying the projected body map.
	var body map[string]any
	if err := json.Unmarshal([]byte(recs[0].Body.StringValue), &body); err != nil {
		t.Fatalf("decode body string: %v", err)
	}
	if body["event"] != "set.success" {
		t.Errorf("body[event] = %v, want %q", body["event"], "set.success")
	}
	if _, has := body["actor.tty"]; has {
		t.Errorf("body should not contain actor.tty")
	}
}

func TestOTLPSink_BatchWindowFlushesIncomplete(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      10,
		BatchWindow:    30 * time.Millisecond,
		QueueCap:       10,
		MaxRetries:     1,
		RetryBaseDelay: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	for i := 0; i < 3; i++ {
		if err := sink.Write(context.Background(), mkEvent(uint64(i+1), "set.success")); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	if !f.waitForRequests(1, 2*time.Second) {
		t.Fatalf("expected at least 1 request via batch window; got %d", f.requestCount())
	}
}

func TestOTLPSink_BatchSizeForcesImmediateFlush(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      2,
		BatchWindow:    time.Hour,
		QueueCap:       10,
		MaxRetries:     1,
		RetryBaseDelay: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	for i := 0; i < 4; i++ {
		if err := sink.Write(context.Background(), mkEvent(uint64(i+1), "set.success")); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	if !f.waitForRequests(2, 2*time.Second) {
		t.Fatalf("expected >= 2 batches; got %d", f.requestCount())
	}
}

func TestOTLPSink_Retries5xxThenSucceeds(t *testing.T) {
	f, srv := startFake(t)
	f.statuses = []int{500, 500, 200}
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      1,
		BatchWindow:    time.Hour,
		QueueCap:       10,
		MaxRetries:     5,
		RetryBaseDelay: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !f.waitForRequests(3, 2*time.Second) {
		t.Fatalf("expected 3 attempts (500,500,200); got %d", f.requestCount())
	}
	if sink.Dropped() != 0 {
		t.Errorf("Dropped = %d, want 0 (eventual success)", sink.Dropped())
	}
}

func TestOTLPSink_RetriesExhaustedDropsBatch(t *testing.T) {
	f, srv := startFake(t)
	f.defaultStatus = 500
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      1,
		BatchWindow:    time.Hour,
		QueueCap:       10,
		MaxRetries:     2,
		RetryBaseDelay: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// 1 initial + 2 retries = 3 total attempts.
	if !f.waitForRequests(3, 2*time.Second) {
		t.Fatalf("expected 3 attempts; got %d", f.requestCount())
	}
	// Wait for drop to register.
	deadline := time.Now().Add(2 * time.Second)
	for sink.Dropped() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if sink.Dropped() == 0 {
		t.Errorf("expected Dropped > 0 after exhausting retries")
	}
}

func TestOTLPSink_4xxDropsImmediately(t *testing.T) {
	f, srv := startFake(t)
	f.defaultStatus = 400
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      1,
		BatchWindow:    time.Hour,
		QueueCap:       10,
		MaxRetries:     5,
		RetryBaseDelay: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !f.waitForRequests(1, 2*time.Second) {
		t.Fatalf("expected 1 attempt")
	}
	// Give it a moment to verify no retries happen.
	time.Sleep(50 * time.Millisecond)
	if got := f.requestCount(); got != 1 {
		t.Errorf("requestCount = %d, want 1 (no retries on 4xx)", got)
	}
	if sink.Dropped() == 0 {
		t.Errorf("expected Dropped > 0 on 4xx")
	}
}

func TestOTLPSink_LocalOnlyEventsNeverSent(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   1,
		BatchWindow: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	defer sink.Close()

	e := mkEvent(1, "set.success")
	e.LocalOnly = true
	if err := sink.Write(context.Background(), e); err != nil {
		t.Fatalf("Write: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := f.requestCount(); got != 0 {
		t.Errorf("requestCount = %d, want 0 (LocalOnly)", got)
	}
}

func TestOTLPSink_AuditPrefixEventsNeverSent(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   1,
		BatchWindow: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	defer sink.Close()

	if err := sink.Write(context.Background(), mkEvent(1, "audit.verify.failed")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := f.requestCount(); got != 0 {
		t.Errorf("requestCount = %d, want 0 (audit.*)", got)
	}
}

func TestOTLPSink_QueueFullDrops(t *testing.T) {
	f, srv := startFake(t)
	f.hang = true // first request blocks until release closed
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   1,
		BatchWindow: time.Hour,
		QueueCap:    2,
		MaxRetries:  1,
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() {
		// Release any hung request, then close.
		select {
		case <-f.release:
		default:
			close(f.release)
		}
		_ = sink.Close()
	})

	// Pump many events. The writer goroutine grabs the first one and
	// blocks on the hanging HTTP request, so the channel fills and
	// later writes drop.
	const total = 100
	for i := 0; i < total; i++ {
		_ = sink.Write(context.Background(), mkEvent(uint64(i+1), "set.success"))
	}
	// Wait until at least one Drop is recorded.
	deadline := time.Now().Add(2 * time.Second)
	for sink.Dropped() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if sink.Dropped() == 0 {
		t.Fatalf("expected Dropped > 0 with full queue")
	}
}

func TestOTLPSink_CloseFlushesPending(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   100,
		BatchWindow: time.Hour, // only Close should cause flush
		QueueCap:    10,
		MaxRetries:  1,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := sink.Write(context.Background(), mkEvent(uint64(i+1), "set.success")); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := f.requestCount(); got < 1 {
		t.Errorf("Close should have flushed pending; requestCount = %d", got)
	}
}

func TestOTLPSink_CloseTimesOutOnSlowServer(t *testing.T) {
	f, srv := startFake(t)
	f.hang = true
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:       srv.URL,
		Transformer:    mkTransformer(),
		BatchSize:      1,
		BatchWindow:    time.Hour,
		QueueCap:       10,
		MaxRetries:     10,
		RetryBaseDelay: 1 * time.Hour, // make retries effectively infinite during close
		HTTPClient:     &http.Client{Timeout: time.Hour},
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Wait for the writer goroutine to start the hanging HTTP request
	// before we Close, so we exercise the close-timeout branch.
	if !f.waitForRequests(1, 2*time.Second) {
		t.Fatalf("expected first attempt to reach server before Close")
	}

	// Override the package-level closeFlushDeadline for this test is
	// not possible without state injection; instead we just verify the
	// Close call returns within a reasonable wall-clock window.
	doneCh := make(chan error, 1)
	go func() { doneCh <- sink.Close() }()

	select {
	case err := <-doneCh:
		// Either timeout error or nil if it managed to finish — both
		// are bounded outcomes; we mainly assert Close returned
		// promptly relative to RetryBaseDelay (1h).
		if err == nil {
			t.Logf("Close returned nil (writer drained)")
		}
	case <-time.After(closeFlushDeadline + 2*time.Second):
		t.Fatalf("Close did not return within close deadline window")
	}

	// Release the hung request so the server can close cleanly.
	select {
	case <-f.release:
	default:
		close(f.release)
	}
}

func TestOTLPSink_HeadersPropagated(t *testing.T) {
	f, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		Headers: map[string]string{
			"Authorization": "Bearer test-token",
			"X-Custom":      "yes",
		},
		BatchSize:   1,
		BatchWindow: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reqs := f.snapshot()
	if len(reqs) == 0 {
		t.Fatalf("no requests captured")
	}
	if got := reqs[0].Headers.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := reqs[0].Headers.Get("X-Custom"); got != "yes" {
		t.Errorf("X-Custom = %q, want %q", got, "yes")
	}
	if got := reqs[0].Headers.Get("User-Agent"); got == "" {
		t.Errorf("User-Agent missing")
	}
}

func TestOTLPSink_WriteAfterCloseErrors(t *testing.T) {
	_, srv := startFake(t)
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   1,
		BatchWindow: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := sink.Write(context.Background(), mkEvent(1, "set.success")); err == nil {
		t.Errorf("Write after Close should error")
	}
}

// Sanity check that Dropped is safe for concurrent reads while the
// writer might be incrementing it. The race detector will catch any
// foot-guns here.
func TestOTLPSink_DroppedReadConcurrentSafe(t *testing.T) {
	f, srv := startFake(t)
	f.hang = true
	sink, err := NewOTLPSink(OTLPSinkConfig{
		Endpoint:    srv.URL,
		Transformer: mkTransformer(),
		BatchSize:   1,
		BatchWindow: time.Hour,
		QueueCap:    1,
		MaxRetries:  1,
	})
	if err != nil {
		t.Fatalf("NewOTLPSink: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-f.release:
		default:
			close(f.release)
		}
		_ = sink.Close()
	})

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_ = sink.Dropped()
		}
	}()
	for i := 0; i < 200; i++ {
		_ = sink.Write(context.Background(), mkEvent(uint64(i+1), "set.success"))
	}
	stop.Store(true)
	wg.Wait()
}
