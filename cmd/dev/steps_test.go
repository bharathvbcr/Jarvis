package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func step(id, lane string, needs ...string) testStep {
	return testStep{id: id, name: lane, needs: needs}
}

// recorder notes when steps start and finish and how many of a lane ran at once.
type recorder struct {
	mu       sync.Mutex
	order    []string
	finished map[string]bool
	live     map[string]int
	peak     map[string]int
}

func newRecorder() *recorder {
	return &recorder{finished: map[string]bool{}, live: map[string]int{}, peak: map[string]int{}}
}

func (r *recorder) enter(s testStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, s.id)
	r.live[s.name]++
	r.peak[s.name] = max(r.peak[s.name], r.live[s.name])
}

func (r *recorder) leave(s testStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live[s.name]--
	r.finished[s.id] = true
}

func (r *recorder) done(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finished[id]
}

// A Go step and a cargo step must be in flight together: each waits for the
// other to start, which a one-at-a-time run can never satisfy.
func TestRunStepsOverlapsStepsOfDifferentLanes(t *testing.T) {
	var started [2]chan struct{}
	for i := range started {
		started[i] = make(chan struct{})
	}
	steps := []testStep{step("go", "go"), step("rust", "cargo")}
	run := func(ctx context.Context, s testStep, _ io.Writer) error {
		me, other := 0, 1
		if s.id == "rust" {
			me, other = 1, 0
		}
		close(started[me])
		select {
		case <-started[other]:
			return nil
		case <-time.After(5 * time.Second):
			return errors.New("the other lane never started: steps ran one at a time")
		}
	}
	if err := runSteps(context.Background(), steps, false, io.Discard, run); err != nil {
		t.Fatal(err)
	}
	// The same plan with --serial must not overlap, so the same wait times out.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	shortRun := func(ctx context.Context, s testStep, _ io.Writer) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return errors.New("never cancelled")
		}
	}
	rec := newRecorder()
	err := runSteps(ctx, steps, true, io.Discard, func(c context.Context, s testStep, w io.Writer) error {
		rec.enter(s)
		defer rec.leave(s)
		return shortRun(c, s, w)
	})
	if err == nil || !slices.Equal(rec.order, []string{"go"}) {
		t.Fatalf("serial plan should have stopped inside its first step, ran %v, err %v", rec.order, err)
	}
}

func TestRunStepsRunsOneStepPerLaneInListedOrder(t *testing.T) {
	steps := []testStep{step("g1", "go"), step("c1", "cargo"), step("g2", "go"), step("c2", "cargo"), step("g3", "go"), step("c3", "cargo")}
	rec := newRecorder()
	run := func(_ context.Context, s testStep, _ io.Writer) error {
		rec.enter(s)
		time.Sleep(15 * time.Millisecond)
		rec.leave(s)
		return nil
	}
	if err := runSteps(context.Background(), steps, false, io.Discard, run); err != nil {
		t.Fatal(err)
	}
	if rec.peak["go"] != 1 || rec.peak["cargo"] != 1 {
		t.Fatalf("more than one step of a lane ran at once: %v", rec.peak)
	}
	for _, lane := range [][]string{{"g1", "g2", "g3"}, {"c1", "c2", "c3"}} {
		var got []string
		for _, id := range rec.order {
			if slices.Contains(lane, id) {
				got = append(got, id)
			}
		}
		if !slices.Equal(got, lane) {
			t.Fatalf("lane ran %v, want %v", got, lane)
		}
	}
}

func TestRunStepsSerialIsPlanOrderOneAtATime(t *testing.T) {
	steps := []testStep{step("a", "go"), step("b", "cargo"), step("c", "go"), step("d", "cargo")}
	var running atomic.Int32
	var peak atomic.Int32
	var order []string
	var mu sync.Mutex
	run := func(_ context.Context, s testStep, _ io.Writer) error {
		n := running.Add(1)
		defer running.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		mu.Lock()
		order = append(order, s.id)
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return nil
	}
	if err := runSteps(context.Background(), steps, true, io.Discard, run); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 1 || !slices.Equal(order, []string{"a", "b", "c", "d"}) {
		t.Fatalf("serial run overlapped or reordered: peak %d, order %v", peak.Load(), order)
	}
}

func TestRunStepsStartsAStepOnlyAfterItsNeedsPassed(t *testing.T) {
	steps := []testStep{step("verify", "cargo", "host"), step("host", "go")}
	rec := newRecorder()
	run := func(_ context.Context, s testStep, _ io.Writer) error {
		rec.enter(s)
		defer rec.leave(s)
		if s.id == "host" {
			time.Sleep(80 * time.Millisecond)
			return nil
		}
		if !rec.done("host") {
			return errors.New("verify started before the host build finished")
		}
		return nil
	}
	if err := runSteps(context.Background(), steps, false, io.Discard, run); err != nil {
		t.Fatal(err)
	}
}

func TestRunStepsFailureCancelsRunningStepsAndStartsNoMore(t *testing.T) {
	steps := []testStep{step("boom", "go"), step("slow", "cargo"), step("later", "cargo"), step("after-boom", "go")}
	var log bytes.Buffer
	var ran sync.Map
	run := func(ctx context.Context, s testStep, _ io.Writer) error {
		ran.Store(s.id, true)
		switch s.id {
		case "boom":
			time.Sleep(30 * time.Millisecond)
			return errors.New("exit status 1")
		case "slow":
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return errors.New("slow step was not cancelled")
			}
		}
		return nil
	}
	start := time.Now()
	err := runSteps(context.Background(), steps, false, &log, run)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("error = %v, want the first failure, named", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the failure did not cancel the running step")
	}
	for _, id := range []string{"later", "after-boom"} {
		if _, ok := ran.Load(id); ok {
			t.Fatalf("%s started after a failure", id)
		}
	}
	// A step that was only cancelled must not be reported as a second failure.
	summary := log.String()
	for _, want := range []string{"boom", stepFailed, "slow", stepCancelled, "later", stepSkipped} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary lacks %q:\n%s", want, summary)
		}
	}
	if strings.Count(summary, stepFailed) != 1 {
		t.Fatalf("summary counts more than one failure:\n%s", summary)
	}
}

func TestRunStepsReportsAnInterruptAsIncomplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := func(ctx context.Context, s testStep, _ io.Writer) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	if err := runSteps(ctx, []testStep{step("a", "go")}, false, io.Discard, run); err == nil {
		t.Fatal("an interrupted plan reported success")
	}
}

func TestRunStepsRefusesAnUnrunnablePlan(t *testing.T) {
	cases := map[string][]testStep{
		"missing id":        {{name: "go"}},
		"duplicate id":      {step("a", "go"), step("a", "cargo")},
		"unknown need":      {step("a", "go", "nope")},
		"cycle across lane": {step("a", "go", "b"), step("b", "cargo", "a")},
		// c waits on d, but d runs after c in the same lane: it could never start.
		"cycle through lane order": {step("c", "go", "d"), step("d", "go")},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			err := runSteps(context.Background(), steps, false, io.Discard, func(context.Context, testStep, io.Writer) error {
				called = true
				return nil
			})
			if err == nil || called {
				t.Fatalf("plan was accepted (err %v, ran a step: %v)", err, called)
			}
		})
	}
	// Serial puts every step in one lane, so a need on a later step is a cycle there.
	late := []testStep{step("a", "go", "b"), step("b", "cargo")}
	if err := validateSteps(late, false); err != nil {
		t.Fatalf("overlapping plan refused: %v", err)
	}
	if err := validateSteps(late, true); err == nil {
		t.Fatal("serial plan with a need on a later step accepted")
	}
}

func TestTestStepsPlanIsRunnableAndOrdersTheHostBeforeDcVerify(t *testing.T) {
	steps := testSteps("/j", "/j/.local/upstream/Manvi", "/j/.local/upstream/DevCouncil", "/tmp/h/devcouncil", nil)
	for _, serial := range []bool{false, true} {
		if err := validateSteps(steps, serial); err != nil {
			t.Fatalf("serial=%v: %v", serial, err)
		}
	}
	byID := map[string]testStep{}
	for _, s := range steps {
		byID[s.id] = s
	}
	if v := byID["cargo-dc-verify"]; !slices.Contains(v.needs, "devcouncil-host") {
		t.Fatalf("dc-verify does not wait for the host build: %+v", v)
	}
	// go-jarvis runs cargo (the cmd/dev fixture builds), so a second cargo step
	// beside it would break the one-cargo-job rule the lanes exist for.
	if d := byID["cargo-desktop"]; !slices.Contains(d.needs, "go-jarvis") {
		t.Fatalf("cargo-desktop may run beside go-jarvis, which runs cargo: %+v", d)
	}
	if b := byID["qualify-boundaries"]; b.name != "go" || !slices.Contains(b.args, "--check-boundaries") {
		t.Fatalf("boundary check is not in the plan: %+v", b)
	}
}

func TestLineWriterKeepsLinesWholeAcrossConcurrentWriters(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l := &lineWriter{mu: &mu, out: &out, prefix: fmt.Sprintf("[w%d] ", w)}
			for i := 0; i < 200; i++ {
				// Split each line across two writes, as a pipe may.
				fmt.Fprintf(l, "line %d ", i)
				fmt.Fprintf(l, "end\n")
			}
			if err := l.flush(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 800 {
		t.Fatalf("got %d lines, want 800", len(lines))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "[w") || !strings.HasSuffix(line, " end") || strings.Count(line, "[w") != 1 {
			t.Fatalf("line was split or merged: %q", line)
		}
	}
}

func TestLineWriterFlushesAnUnterminatedLastLineAndBoundsLongOnes(t *testing.T) {
	var out bytes.Buffer
	var mu sync.Mutex
	l := &lineWriter{mu: &mu, out: &out, prefix: "> "}
	io.WriteString(l, "tail without newline")
	if err := l.flush(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "> tail without newline\n" {
		t.Fatalf("got %q", out.String())
	}
	out.Reset()
	io.WriteString(l, strings.Repeat("x", maxLine*2+10))
	if len(l.buf) >= maxLine {
		t.Fatalf("held %d bytes of one unterminated line", len(l.buf))
	}
	l.flush()
	if got := strings.Count(out.String(), "\n"); got != 3 {
		t.Fatalf("a %d-byte line became %d lines, want 3", maxLine*2+10, got)
	}
	if strings.Count(out.String(), "x") != maxLine*2+10 {
		t.Fatal("bytes were lost cutting a long line")
	}
}

// A real subprocess through the real executor: its output reaches the log with
// the step's id on every line, CGO_ENABLED is 0 unless the step's env says
// otherwise, and a non-zero exit comes back as the step's failure.
func TestExecTestStepRunsRealCommandsWithTheStepEnvironment(t *testing.T) {
	var log bytes.Buffer
	steps := []testStep{
		{id: "default-env", dir: t.TempDir(), name: "go", args: []string{"env", "CGO_ENABLED"}},
		{id: "step-env-wins", dir: t.TempDir(), name: "go", args: []string{"env", "CGO_ENABLED"}, env: []string{"CGO_ENABLED=1"}},
	}
	if err := runSteps(context.Background(), steps, true, &log, execTestStep); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	for _, want := range []string{"[default-env] 0\n", "[step-env-wins] 1\n"} {
		if !strings.Contains(log.String(), want) {
			t.Fatalf("log lacks %q:\n%s", want, log.String())
		}
	}
	log.Reset()
	bad := []testStep{{id: "bad", dir: t.TempDir(), name: "go", args: []string{"definitely-not-a-go-command"}}}
	err := runSteps(context.Background(), bad, false, &log, execTestStep)
	if err == nil || !strings.Contains(err.Error(), "bad") || !strings.Contains(log.String(), "[bad] ") {
		t.Fatalf("a failing command was not reported: err %v\n%s", err, log.String())
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("log closed") }

func TestLineWriterReportsALogWriteFailure(t *testing.T) {
	var mu sync.Mutex
	l := &lineWriter{mu: &mu, out: failWriter{}, prefix: "> "}
	if _, err := io.WriteString(l, "a\n"); err == nil {
		t.Fatal("a failed log write was swallowed")
	}
}
