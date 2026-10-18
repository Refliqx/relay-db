// Package jsonrpc implements JSON-RPC 2.0 over NDJSON (newline delimited JSON)
// on stdin/stdout. NDJSON was chosen because:
//  1. it needs no LSP-style Content-Length framing,
//  2. Go's encoding/json always escapes newlines inside strings, so
//     1 frame == 1 line by construction,
//  3. it can be tailed like a log while debugging (`relaydb-engine | jq`).
package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
)

type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Error is a JSON-RPC error with a domain code.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

func NewError(code int, msg string) *Error { return &Error{Code: code, Message: msg} }

func (e *Error) WithData(d any) *Error { e.Data = d; return e }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Server is the JSON-RPC dispatcher.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]Handler

	enc     *json.Encoder
	writeMu sync.Mutex

	// inflight tracks active requests to support $/cancelRequest.
	inflight   map[string]context.CancelFunc
	inflightMu sync.Mutex
	seqID      atomic.Uint64

	log *slog.Logger
}

func NewServer(w io.Writer, log *slog.Logger) *Server {
	enc := json.NewEncoder(w)
	// Go's encoder appends '\n' after every Encode() -> NDJSON.
	return &Server{
		handlers: make(map[string]Handler),
		enc:      enc,
		inflight: make(map[string]context.CancelFunc),
		log:      log,
	}
}

func (s *Server) Register(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Notify sends a notification (fire-and-forget) to the client.
func (s *Server) Notify(method string, params any) error {
	return s.write(notification{JSONRPC: "2.0", Method: method, Params: params})
}

func (s *Server) write(v any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.enc.Encode(v)
}

// Serve reads NDJSON from r until EOF / ctx is cancelled.
// Handlers run on a separate goroutine so a long request (e.g. export.run)
// does not block other requests such as export.cancel.
func (s *Server) Serve(ctx context.Context, r io.Reader) error {
	sc := bufio.NewScanner(r)
	// Large buffer: one line can hold a schema snapshot tens of MB in size.
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	var wg sync.WaitGroup
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		raw := make([]byte, len(line))
		copy(raw, line)

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.dispatch(ctx, raw)
		}()
	}
	wg.Wait()
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("reading stdin: %w", err)
	}
	return nil
}

func (s *Server) dispatch(parent context.Context, raw []byte) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = s.write(response{JSONRPC: "2.0", Error: NewError(-32700, "parse error: "+err.Error())})
		return
	}

	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	if req.Method == "$/cancelRequest" {
		s.handleCancel(req.Params)
		return
	}

	s.mu.RLock()
	h, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	if !ok {
		if !isNotification {
			_ = s.write(response{JSONRPC: "2.0", ID: req.ID, Error: NewError(-32601, "method not found: "+req.Method)})
		}
		return
	}

	key := string(req.ID)
	ctx := parent
	var cancel context.CancelFunc
	if !isNotification {
		ctx, cancel = context.WithCancel(parent)
		s.inflightMu.Lock()
		s.inflight[key] = cancel
		s.inflightMu.Unlock()
		defer func() {
			s.inflightMu.Lock()
			delete(s.inflight, key)
			s.inflightMu.Unlock()
			cancel()
		}()
	}

	result, err := h(ctx, req.Params)
	if isNotification {
		return
	}
	if err != nil {
		var rpcErr *Error
		if errors.As(err, &rpcErr) {
			_ = s.write(response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
			return
		}
		if errors.Is(err, context.Canceled) {
			_ = s.write(response{JSONRPC: "2.0", ID: req.ID, Error: NewError(-32010, "cancelled")})
			return
		}
		_ = s.write(response{JSONRPC: "2.0", ID: req.ID, Error: NewError(-32603, err.Error())})
		return
	}
	if err := s.write(response{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
		s.log.Error("failed to write response", "err", err)
	}
}

type cancelParams struct {
	ID json.RawMessage `json:"id"`
}

func (s *Server) handleCancel(params json.RawMessage) {
	var p cancelParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	s.inflightMu.Lock()
	cancel, ok := s.inflight[string(p.ID)]
	s.inflightMu.Unlock()
	if ok {
		s.log.Info("cancelling request", "id", string(p.ID))
		cancel()
	}
}
