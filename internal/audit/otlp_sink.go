// Copyright (c) 2026, Logan Lindquist Land
// SPDX-License-Identifier: BSD-3-Clause

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llbbl/lsm/internal/dlog"
)

// Default tunables for OTLPSink. See OTLPSinkConfig for usage.
const (
	defaultBatchSize      = 100
	defaultBatchWindow    = 5 * time.Second
	defaultQueueCap       = 1000
	defaultMaxRetries     = 3
	defaultRetryBaseDelay = 1 * time.Second
	defaultHTTPTimeout    = 10 * time.Second
	closeFlushDeadline    = 5 * time.Second
	userAgent             = "lsm-audit/dev"
)

// OTLPSinkConfig configures an OTLPSink. All fields except Endpoint and
// Transformer.Salt have sensible defaults.
type OTLPSinkConfig struct {
	Endpoint       string            // required, e.g. "https://otlp.example.com/v1/logs"
	Headers        map[string]string // optional, e.g. {"Authorization": "Bearer xxx"}
	BatchSize      int               // events per batch (default 100)
	BatchWindow    time.Duration     // flush after this long even if batch incomplete (default 5s)
	QueueCap       int               // bounded channel cap (default 1000)
	MaxRetries     int               // per-batch (default 3)
	RetryBaseDelay time.Duration     // first retry delay; doubles each attempt (default 1s)
	HTTPClient     *http.Client      // optional; defaults to a 10s timeout client

	Transformer Transformer  // required (Salt + HostName)
	Logger      *slog.Logger // optional, for drop warnings; uses dlog.Discard if nil
}

// OTLPSink is a Sink that asynchronously batches and ships Events over
// OTLP/HTTP. Local-only events (LocalOnly==true, or event name starting
// with "audit.") are dropped silently. Other events are projected via
// the Transformer and queued for the writer goroutine.
type OTLPSink struct {
	cfg     OTLPSinkConfig
	queue   chan *ProjectedLog
	done    chan struct{}
	wg      sync.WaitGroup
	logger  *slog.Logger
	dropped atomic.Int64

	closeOnce sync.Once
	closeErr  error
}

// NewOTLPSink constructs an OTLPSink, validating config and starting
// the writer goroutine.
func NewOTLPSink(cfg OTLPSinkConfig) (*OTLPSink, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("audit: OTLPSink requires Endpoint")
	}
	if cfg.Transformer.Salt == nil {
		return nil, errors.New("audit: OTLPSink requires Transformer.Salt")
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.BatchWindow <= 0 {
		cfg.BatchWindow = defaultBatchWindow
	}
	if cfg.QueueCap <= 0 {
		cfg.QueueCap = defaultQueueCap
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.RetryBaseDelay <= 0 {
		cfg.RetryBaseDelay = defaultRetryBaseDelay
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = dlog.Discard
	}

	s := &OTLPSink{
		cfg:    cfg,
		queue:  make(chan *ProjectedLog, cfg.QueueCap),
		done:   make(chan struct{}),
		logger: logger,
	}
	s.wg.Add(1)
	go s.run()
	return s, nil
}

// Write conforms to Sink. Drops local-only events. Projects + enqueues
// other events. Never blocks: if the queue is full, the event is
// dropped and the dropped counter is incremented.
func (s *OTLPSink) Write(_ context.Context, e Event) error {
	select {
	case <-s.done:
		return errors.New("audit: OTLPSink is closed")
	default:
	}

	proj, ok := s.cfg.Transformer.Project(e)
	if !ok {
		return nil
	}

	select {
	case s.queue <- proj:
		return nil
	default:
		s.dropped.Add(1)
		s.logger.Warn("audit: otlp queue full, dropping event",
			"event", e.Event,
			"seq", e.Seq)
		return nil
	}
}

// Close stops the writer goroutine cleanly, flushing whatever's queued
// up to a reasonable deadline. Idempotent.
func (s *OTLPSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		doneWait := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(doneWait)
		}()
		select {
		case <-doneWait:
		case <-time.After(closeFlushDeadline):
			s.closeErr = fmt.Errorf("audit: OTLPSink close timed out; some events may be unflushed")
		}
	})
	return s.closeErr
}

// Dropped returns the running count of events the sink had to drop
// because the queue was full.
func (s *OTLPSink) Dropped() int64 {
	return s.dropped.Load()
}

// run is the single writer goroutine. It maintains a buffer of pending
// projections, flushing on BatchSize, BatchWindow tick, or close.
func (s *OTLPSink) run() {
	defer s.wg.Done()

	buf := make([]*ProjectedLog, 0, s.cfg.BatchSize)
	ticker := time.NewTicker(s.cfg.BatchWindow)
	defer ticker.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}
		s.send(buf)
		buf = buf[:0]
	}

	for {
		select {
		case <-s.done:
			// Drain queued items, then final flush.
			for {
				select {
				case p := <-s.queue:
					buf = append(buf, p)
					if len(buf) >= s.cfg.BatchSize {
						s.send(buf)
						buf = buf[:0]
					}
				default:
					flush()
					return
				}
			}
		case p := <-s.queue:
			buf = append(buf, p)
			if len(buf) >= s.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// send POSTs a batch as OTLP/HTTP JSON, retrying transient failures
// with exponential backoff up to MaxRetries.
func (s *OTLPSink) send(batch []*ProjectedLog) {
	body, err := s.encodeBatch(batch)
	if err != nil {
		s.logger.Warn("audit: otlp encode failed, dropping batch", "error", err, "count", len(batch))
		s.dropped.Add(int64(len(batch)))
		return
	}

	delay := s.cfg.RetryBaseDelay
	for attempt := 0; attempt <= s.cfg.MaxRetries; attempt++ {
		status, err := s.post(body)
		if err == nil && status >= 200 && status < 300 {
			return
		}
		// 4xx: client error, no point retrying.
		if err == nil && status >= 400 && status < 500 {
			s.logger.Warn("audit: otlp 4xx response, dropping batch",
				"status", status,
				"count", len(batch))
			s.dropped.Add(int64(len(batch)))
			return
		}
		// 5xx or transport error: retry if we have budget.
		if attempt == s.cfg.MaxRetries {
			s.logger.Warn("audit: otlp exhausted retries, dropping batch",
				"status", status,
				"error", err,
				"count", len(batch))
			s.dropped.Add(int64(len(batch)))
			return
		}
		select {
		case <-time.After(delay):
		case <-s.done:
			// Closing — abandon retries.
			s.logger.Warn("audit: otlp closing while retrying, dropping batch", "count", len(batch))
			s.dropped.Add(int64(len(batch)))
			return
		}
		delay *= 2
	}
}

// post does a single HTTP POST with all configured headers. Returns
// the status code (0 on transport error) and an error.
func (s *OTLPSink) post(body []byte) (int, error) {
	req, err := http.NewRequest(http.MethodPost, s.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	for k, v := range s.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	// Drain response body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// encodeBatch builds the OTLP/HTTP JSON payload for a batch of
// projections. All records share a single resourceLogs / scopeLogs
// envelope; per-record labels live on each record's attributes.
func (s *OTLPSink) encodeBatch(batch []*ProjectedLog) ([]byte, error) {
	records := make([]map[string]any, 0, len(batch))
	now := time.Now().UnixNano()
	for _, p := range batch {
		bodyJSON, err := json.Marshal(p.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		records = append(records, map[string]any{
			"timeUnixNano":   strconv.FormatInt(now, 10),
			"severityNumber": 9,
			"severityText":   "INFO",
			"body": map[string]any{
				"stringValue": string(bodyJSON),
			},
			"attributes": labelsToAttrs(p.Labels),
		})
	}
	payload := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						kvStr("service.name", "lsm"),
						kvStr("host.name", s.cfg.Transformer.HostName),
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope":      map[string]any{"name": "lsm-audit"},
						"logRecords": records,
					},
				},
			},
		},
	}
	return json.Marshal(payload)
}

// labelsToAttrs converts a label map to the OTLP attributes array
// shape. Keys are emitted in a stable order to keep tests
// deterministic.
func labelsToAttrs(labels map[string]string) []any {
	keys := []string{"event", "app", "env", "host", "tty_present", "agent_marker"}
	attrs := make([]any, 0, len(keys))
	for _, k := range keys {
		attrs = append(attrs, kvStr(k, labels[k]))
	}
	return attrs
}

// kvStr produces a single OTLP key-value attribute with a string value.
func kvStr(k, v string) map[string]any {
	return map[string]any{
		"key":   k,
		"value": map[string]any{"stringValue": v},
	}
}

// Compile-time assertion that OTLPSink satisfies Sink.
var _ Sink = (*OTLPSink)(nil)
