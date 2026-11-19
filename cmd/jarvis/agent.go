package main

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/gussetcheck"
	"github.com/bharathvbcr/Manvi/manvi/serve"
)

// Exit codes for agent-facing commands. Every other command keeps exiting 1
// on any failure.
const (
	exitFailed     = 1 // the operation ran and failed, or the run failed
	exitUsage      = 2 // bad arguments, or a request the host refused as malformed
	exitRefused    = 3 // the agent gate refused the op (E_AGENT_REFUSED, E_PROCESS_SCOPED)
	exitNeedsHuman = 4 // a run reached a human gate in --events mode and was cancelled
	exitTimeout    = 5 // --timeout passed before the call finished
)

const (
	errTimeout         = "E_TIMEOUT"
	defaultCallTimeout = 35 * time.Second // above the 30 s ceiling of any background job
	maxCallTimeout     = 10 * time.Minute
	maxParamsBytes     = 1 << 20  // workflow.DecodeStrict's own document bound
	maxResponseLine    = 64 << 20 // above the 16 MiB job-result bound, with JSON overhead
	jobPollInterval    = 100 * time.Millisecond
)

// exitError carries a process exit code. A nil err means the detail was
// already written to stdout.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}
func (e *exitError) Unwrap() error { return e.err }

// envelope is serve.Response as the CLI writes it when the CLI, not the
// host, has the answer: a refusal, a timeout, or a polled job's result.
type envelope struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result jsontext.Value `json:"result,omitzero"`
	Error  *serve.Error   `json:"error,omitzero"`
}

// writeLine writes one compact JSON line with encoding/json/v2 under the v1
// option set, so the bytes match what the host plane itself writes.
func writeLine(w io.Writer, v any) error {
	b, err := jsonv2.Marshal(v, json.DefaultOptionsV1())
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// exitFor maps a host-plane error code to an exit code.
func exitFor(e *serve.Error) int {
	switch e.Code {
	case serve.ErrBadRequest, serve.ErrTooLarge:
		return exitUsage
	case app.ErrAgentRefused, app.ErrProcessScoped:
		return exitRefused
	case errTimeout:
		return exitTimeout
	default:
		return exitFailed
	}
}

// fail writes e as the call's one stdout line and returns its exit.
func fail(stdout io.Writer, id string, e *serve.Error) error {
	if err := writeLine(stdout, envelope{ID: id, Error: e}); err != nil {
		return err
	}
	return &exitError{code: exitFor(e), err: e}
}

// agentCommand runs `jarvis ops` and `jarvis call`: the host-plane
// operations the workbench reaches over `jarvis serve`, one per process,
// through the same server and handlers, gated by app.AdmitAgent.
func agentCommand(ctx context.Context, command string, args []string, stdin io.Reader, stdout io.Writer) error {
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	root := f.String("root", ".", "Jarvis product root")
	timeout := f.Duration("timeout", defaultCallTimeout, "bound on the whole call, including polling a background job")
	params := f.String("params", "", "call: operation params as inline JSON, @FILE, or - for stdin (default {})")
	yolo := f.Bool("yolo", false, "agent yolo mode (also JARVIS_AGENT_MODE=yolo): admit promote and inputs.put; results are recorded as the agent's")
	if err := f.Parse(args); err != nil {
		return &exitError{code: exitUsage, err: err}
	}
	rest := f.Args()
	if command == "call" && len(rest) > 0 {
		// Accept flags after the op too: jarvis call OP --params @p.json.
		op := rest[0]
		if err := f.Parse(rest[1:]); err != nil {
			return &exitError{code: exitUsage, err: err}
		}
		rest = append([]string{op}, f.Args()...)
	}
	id := command
	switch {
	case command == "ops" && len(rest) != 0:
		return fail(stdout, id, refusal(serve.ErrBadRequest, "use: jarvis ops [--root DIR]"))
	case command == "call" && len(rest) != 1:
		return fail(stdout, id, refusal(serve.ErrBadRequest, "use: jarvis call OP [--params JSON|@FILE|-] [--root DIR] [--timeout D]"))
	case *timeout <= 0 || *timeout > maxCallTimeout:
		return fail(stdout, id, refusal(serve.ErrBadRequest, "--timeout must be within (0, %s]", maxCallTimeout))
	}
	mode, err := resolveAgentMode(*yolo)
	if err != nil {
		return fail(stdout, id, refusal(serve.ErrBadRequest, "%v", err))
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	a, err := app.New(ctx, app.Config{Root: absRoot, Operator: operatorFor(mode)})
	if err != nil {
		return err
	}
	defer a.Close()

	var op string
	var raw json.RawMessage
	if command == "call" {
		op = rest[0]
		var inline bool
		if raw, inline, err = readParams(*params, stdin); err != nil {
			return fail(stdout, id, refusal(serve.ErrBadRequest, "--params: %v", err))
		}
		if e := a.AdmitAgent(op, raw, inline, mode); e != nil {
			return fail(stdout, id, e)
		}
	}
	s, err := openSession(ctx, a)
	if err != nil {
		return timedOut(ctx, stdout, id, err)
	}
	callErr := func() error {
		if command == "ops" {
			list, err := encodeValue(struct {
				Mode string       `json:"mode"`
				Ops  []app.OpInfo `json:"ops"`
			}{modeName(mode), app.AgentOps(s.ops)})
			if err != nil {
				return err
			}
			return writeLine(stdout, envelope{ID: id, OK: true, Result: list})
		}
		return s.callOp(ctx, stdout, id, op, raw)
	}()
	return errors.Join(callErr, s.close())
}

// resolveAgentMode is the one place the agent mode is read: --yolo, or
// JARVIS_AGENT_MODE in the environment. internal/app never reads the
// environment, so `jarvis serve` (the workbench host) and cmd/qualify
// cannot inherit the mode.
func resolveAgentMode(yolo bool) (app.AgentMode, error) {
	mode, err := app.ParseAgentMode(os.Getenv("JARVIS_AGENT_MODE"))
	if err != nil {
		return "", fmt.Errorf("JARVIS_AGENT_MODE: %w", err)
	}
	if yolo {
		mode = app.AgentModeYolo
	}
	return mode, nil
}

// operatorFor is the App operator a mode implies. Yolo errs toward "agent":
// a run started in yolo mode is marked agent-operated even at a terminal.
func operatorFor(mode app.AgentMode) string {
	if mode == app.AgentModeYolo {
		return app.OperatorAgent
	}
	return ""
}

func modeName(mode app.AgentMode) string {
	if mode == app.AgentModeYolo {
		return "yolo"
	}
	return "default"
}

func refusal(code, format string, args ...any) *serve.Error {
	return &serve.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// timedOut reports err as E_TIMEOUT when the call's deadline caused it.
func timedOut(ctx context.Context, stdout io.Writer, id string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fail(stdout, id, refusal(errTimeout, "the call did not finish within --timeout: %v", err))
	}
	return err
}

func encodeValue(v any) (jsontext.Value, error) {
	b, err := jsonv2.Marshal(v, json.DefaultOptionsV1())
	return jsontext.Value(b), err
}

// readParams returns the params document and whether it came from argv.
func readParams(spec string, stdin io.Reader) (json.RawMessage, bool, error) {
	var raw []byte
	inline := false
	switch {
	case spec == "":
		return json.RawMessage(`{}`), false, nil
	case spec == "-":
		b, err := io.ReadAll(io.LimitReader(stdin, maxParamsBytes+1))
		if err != nil {
			return nil, false, err
		}
		raw = b
	case strings.HasPrefix(spec, "@"):
		f, err := os.Open(spec[1:])
		if err != nil {
			return nil, false, err
		}
		b, err := io.ReadAll(io.LimitReader(f, maxParamsBytes+1))
		closeErr := f.Close()
		if err = errors.Join(err, closeErr); err != nil {
			return nil, false, err
		}
		raw = b
	default:
		raw, inline = []byte(spec), true
	}
	if len(raw) > maxParamsBytes {
		return nil, false, fmt.Errorf("params exceed %d bytes", maxParamsBytes)
	}
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") || !json.Valid([]byte(trimmed)) {
		return nil, false, errors.New("params must be one JSON object")
	}
	return json.RawMessage(trimmed), inline, nil
}

// session is one in-process host-plane server, spoken to over pipes exactly
// as `jarvis serve` is spoken to over stdio.
type session struct {
	send    *io.PipeWriter
	replies chan []byte
	readErr chan error
	served  chan error
	stop    context.CancelFunc
	ops     []string
}

func openSession(ctx context.Context, a *app.App) (*session, error) {
	input, send := io.Pipe()
	output, receive := io.Pipe()
	server := serve.New(receive, serve.Options{HardRules: true, Modules: []serve.Module{a}})
	sctx, stop := context.WithCancel(ctx)
	s := &session{send: send, replies: make(chan []byte, 1), readErr: make(chan error, 1), served: make(chan error, 1), stop: stop}
	go func() {
		err := server.Serve(sctx, input)
		_ = receive.CloseWithError(io.EOF)
		s.served <- err
	}()
	go func() {
		reader := bufio.NewReaderSize(output, 64<<10)
		for {
			line, err := readLine(reader)
			if err != nil {
				s.readErr <- err
				return
			}
			select {
			case s.replies <- line:
			case <-sctx.Done():
				s.readErr <- sctx.Err()
				return
			}
		}
	}()
	_, hello, err := s.request(ctx, "hello", serve.OpHello, json.RawMessage(`{"protocol":1,"host":"jarvis-call"}`))
	if err == nil && !hello.OK {
		err = hello.Error
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("host plane hello: %w", err), s.close())
	}
	var result serve.HelloResult
	if err = json.Unmarshal(hello.Result, &result); err != nil {
		return nil, errors.Join(fmt.Errorf("host plane hello: %w", err), s.close())
	}
	s.ops = result.Ops
	return s, nil
}

// readLine reads one response line, bounded so a runaway result cannot
// exhaust memory.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxResponseLine {
			return nil, fmt.Errorf("host-plane response exceeds %d bytes", maxResponseLine)
		}
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

// request sends one call and waits for its single response line, returning
// the line's exact bytes alongside its decoded form.
func (s *session) request(ctx context.Context, id, op string, params json.RawMessage) ([]byte, serve.Response, error) {
	line, err := jsonv2.Marshal(serve.Request{ID: id, Op: op, Params: params}, json.DefaultOptionsV1())
	if err != nil {
		return nil, serve.Response{}, err
	}
	written := make(chan error, 1)
	go func() { _, err := s.send.Write(append(line, '\n')); written <- err }()
	select {
	case err := <-written:
		if err != nil {
			return nil, serve.Response{}, err
		}
	case <-ctx.Done():
		return nil, serve.Response{}, ctx.Err()
	}
	select {
	case raw := <-s.replies:
		var response serve.Response
		if err := json.Unmarshal(raw, &response); err != nil {
			return nil, response, fmt.Errorf("host-plane response: %w", err)
		}
		if response.ID != id {
			return nil, response, fmt.Errorf("host-plane response for %q while waiting for %q", response.ID, id)
		}
		return raw, response, nil
	case err := <-s.readErr:
		return nil, serve.Response{}, fmt.Errorf("host plane closed: %w", err)
	case <-ctx.Done():
		return nil, serve.Response{}, ctx.Err()
	}
}

// callOp sends op and writes its one stdout line. A synchronous op's line is
// the host's own bytes. A background op is polled with jarvis.job.get until
// it ends, and its line carries the op's result, not the job record.
func (s *session) callOp(ctx context.Context, stdout io.Writer, id, op string, params json.RawMessage) error {
	line, response, err := s.request(ctx, id, op, params)
	if err != nil {
		return timedOut(ctx, stdout, id, err)
	}
	if !response.OK || !app.AgentAsync(op) {
		if _, err := stdout.Write(line); err != nil {
			return err
		}
		if !response.OK {
			return &exitError{code: exitFor(response.Error), err: response.Error}
		}
		return nil
	}
	var job app.JobView
	if err := json.Unmarshal(response.Result, &job); err != nil || job.JobID == "" {
		return fmt.Errorf("%s answered without a job: %s", op, response.Result)
	}
	query, err := encodeValue(struct {
		ID string `json:"id"`
	}{job.JobID})
	if err != nil {
		return err
	}
	ticker := time.NewTicker(jobPollInterval)
	defer ticker.Stop()
	for poll := 1; ; poll++ {
		select {
		case <-ctx.Done():
			return timedOut(ctx, stdout, id, fmt.Errorf("job %s still %s", job.JobID, job.Status))
		case <-ticker.C:
		}
		_, polled, err := s.request(ctx, "poll-"+strconv.Itoa(poll), "jarvis.job.get", json.RawMessage(query))
		if err != nil {
			return timedOut(ctx, stdout, id, err)
		}
		if !polled.OK {
			return fail(stdout, id, polled.Error)
		}
		if err := json.Unmarshal(polled.Result, &job); err != nil {
			return fmt.Errorf("jarvis.job.get: %w", err)
		}
		switch job.Status {
		case "succeeded":
			return writeLine(stdout, envelope{ID: id, OK: true, Result: jsontext.Value(job.Result)})
		case "failed", "cancelled":
			return fail(stdout, id, refusal("E_JARVIS", "%s %s: %s", op, job.Status, job.Error))
		}
	}
}

// close ends the session the way `jarvis serve` ends at EOF: the server
// drains and the engine's log ring is written to stderr. The engine itself is
// not shut down here: gussetcheck.Shutdown refuses new work process-wide, and
// a one-shot process releases the engine by exiting, as `jarvis doctor` does.
func (s *session) close() error {
	_ = s.send.Close()
	var err error
	select {
	case err = <-s.served:
	case <-time.After(5 * time.Second):
		err = errors.New("host plane did not drain within 5s")
	}
	s.stop()
	_, drainErr := gussetcheck.DrainLogs(os.Stderr)
	return errors.Join(err, drainErr)
}
