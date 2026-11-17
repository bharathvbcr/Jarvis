package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// newCommand builds a command with CGO_ENABLED=0 and extra after it, so extra
// wins: exec keeps the last value of a repeated key.
func newCommand(ctx context.Context, extra []string, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "CGO_ENABLED=0"), extra...)
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

// runCommand runs one command on the caller's terminal streams.
func runCommand(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, extra []string, dir, name string, args ...string) error {
	cmd := newCommand(ctx, extra, dir, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

// execTestStep runs one step of the test plan. Steps never read stdin: more
// than one runs at once, and none of them asks a question. Each runs in its own
// process group, so cancelling it stops everything it started.
func execTestStep(ctx context.Context, s testStep, w io.Writer) error {
	cmd := newCommand(ctx, s.env, s.dir, s.name, s.args...)
	cmd.Stdout, cmd.Stderr = w, w
	ownProcessGroup(cmd)
	err := cmd.Run()
	// exec fails a command that exited 0 when something it started still holds
	// its output pipe after WaitDelay. The step's own result was a pass, so it
	// stays one, but the leftover is named and stopped, not orphaned.
	if errors.Is(err, exec.ErrWaitDelay) {
		_, _ = io.WriteString(w, "dev: the step exited 0 but left a process holding its output; stopped it\n")
		err = nil
	}
	stopProcessGroup(cmd)
	return err
}

// stepLane is the resource a step is expected to saturate: the intent is one
// Go step beside one cargo step. Cargo parallelizes its own compile across
// cores, so a second cargo step is expected to add queueing rather than speed
// (and to queue behind a host's build-slot gate); a Go step is expected to
// leave room for it. Neither is measured here. One step runs per lane at a time.
func stepLane(s testStep, serial bool) string {
	if serial {
		return ""
	}
	return s.name
}

// validateSteps refuses a plan that could not finish: a missing or repeated
// id, a need that names no step, or a cycle. A cycle includes the one that
// lane order creates, where a step waits on a later step of its own lane.
func validateSteps(steps []testStep, serial bool) error {
	index := make(map[string]int, len(steps))
	for i, s := range steps {
		if s.id == "" {
			return fmt.Errorf("step %d (%s %s) has no id", i, s.name, strings.Join(s.args, " "))
		}
		if _, dup := index[s.id]; dup {
			return fmt.Errorf("step id %q is used twice", s.id)
		}
		index[s.id] = i
	}
	before := make([][]int, len(steps))
	prev := map[string]int{}
	for i, s := range steps {
		for _, n := range s.needs {
			j, ok := index[n]
			if !ok {
				return fmt.Errorf("step %q needs %q, which is not in the plan", s.id, n)
			}
			before[i] = append(before[i], j)
		}
		lane := stepLane(s, serial)
		if j, ok := prev[lane]; ok {
			before[i] = append(before[i], j)
		}
		prev[lane] = i
	}
	left := make([]int, len(steps))
	after := make([][]int, len(steps))
	for i, deps := range before {
		left[i] = len(deps)
		for _, j := range deps {
			after[j] = append(after[j], i)
		}
	}
	var ready []int
	for i, n := range left {
		if n == 0 {
			ready = append(ready, i)
		}
	}
	seen := 0
	for len(ready) > 0 {
		i := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		seen++
		for _, k := range after[i] {
			if left[k]--; left[k] == 0 {
				ready = append(ready, k)
			}
		}
	}
	if seen != len(steps) {
		return errors.New("the test plan has a dependency cycle (counting the order of steps within one lane)")
	}
	return nil
}

// A step's state once the plan is done.
const (
	stepOK        = "ok"
	stepFailed    = "FAILED"
	stepCancelled = "cancelled"
	stepSkipped   = "not run"
)

type stepResult struct {
	status string
	wall   time.Duration
}

// runSteps runs the plan: one step per lane at a time, a step only after
// every step it needs succeeded, and nothing new after the first failure.
// A failure cancels the steps still running. Output reaches log line by line
// with the step's id in front, so it stays live and attributable. serial puts
// every step in one lane, which is plan order. The summary is written whether
// the plan passed or not.
func runSteps(ctx context.Context, steps []testStep, serial bool, log io.Writer, run func(context.Context, testStep, io.Writer) error) error {
	if err := validateSteps(steps, serial); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lanes [][]int
	laneAt := map[string]int{}
	for i, s := range steps {
		key := stepLane(s, serial)
		at, ok := laneAt[key]
		if !ok {
			at = len(lanes)
			laneAt[key] = at
			lanes = append(lanes, nil)
		}
		lanes[at] = append(lanes[at], i)
	}
	passed := make(map[string]chan struct{}, len(steps))
	for _, s := range steps {
		passed[s.id] = make(chan struct{})
	}

	var mu sync.Mutex // serializes writes to log and guards first
	var first error
	fail := func(err error) {
		mu.Lock()
		if first == nil {
			first = err
		}
		mu.Unlock()
		cancel()
	}
	results := make([]stepResult, len(steps))
	for i := range results {
		results[i].status = stepSkipped
	}
	begin := time.Now()
	var wg sync.WaitGroup
	for _, lane := range lanes {
		wg.Add(1)
		go func(lane []int) {
			defer wg.Done()
			for _, i := range lane {
				s := steps[i]
				if !awaitSteps(ctx, passed, s.needs) {
					continue
				}
				out := &lineWriter{mu: &mu, out: log, prefix: "[" + s.id + "] "}
				out.emit([]byte("start\n"))
				start := time.Now()
				err := run(ctx, s, out)
				if flushErr := out.flush(); err == nil {
					err = flushErr
				}
				wall := time.Since(start).Round(time.Millisecond)
				switch {
				case err == nil:
					results[i] = stepResult{stepOK, wall}
					close(passed[s.id])
				case ctx.Err() != nil && func() bool { mu.Lock(); defer mu.Unlock(); return first != nil }():
					results[i] = stepResult{stepCancelled, wall}
				default:
					results[i] = stepResult{stepFailed, wall}
					fail(fmt.Errorf("%s: %w", s.id, err))
				}
				if err != nil {
					return
				}
			}
		}(lane)
	}
	wg.Wait()

	var sum time.Duration
	var table strings.Builder
	fmt.Fprintf(&table, "%-24s %-6s %-10s %s\n", "step", "lane", "result", "wall")
	for i, s := range steps {
		sum += results[i].wall
		fmt.Fprintf(&table, "%-24s %-6s %-10s %s\n", s.id, s.name, results[i].status, results[i].wall)
	}
	fmt.Fprintf(&table, "elapsed %s; the steps took %s in total\n", time.Since(begin).Round(time.Millisecond), sum)
	mu.Lock()
	_, werr := io.WriteString(log, table.String())
	err := first
	mu.Unlock()
	if err == nil && ctx.Err() == nil {
		return werr
	}
	if err == nil {
		// Interrupted from outside: nothing failed, but the plan did not finish.
		return context.Cause(ctx)
	}
	return err
}

// awaitSteps blocks until every named step has passed, and reports false if
// the plan is cancelled first.
func awaitSteps(ctx context.Context, passed map[string]chan struct{}, ids []string) bool {
	for _, id := range ids {
		select {
		case <-passed[id]:
		case <-ctx.Done():
			return false
		}
	}
	return ctx.Err() == nil
}

// lineWriter writes whole lines, each led by prefix, to out under mu, so
// output from steps that run at once never interleaves inside a line. A line
// longer than maxLine is cut there rather than held without bound.
type lineWriter struct {
	mu     *sync.Mutex
	out    io.Writer
	prefix string
	buf    []byte
	err    error
}

const maxLine = 64 << 10

func (l *lineWriter) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			if len(l.buf) < maxLine {
				break
			}
			l.emit(append(l.buf[:maxLine:maxLine], '\n'))
			l.buf = l.buf[maxLine:]
			continue
		}
		l.emit(l.buf[:i+1])
		l.buf = l.buf[i+1:]
	}
	return len(p), l.err
}

func (l *lineWriter) emit(line []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := io.WriteString(l.out, l.prefix); err != nil && l.err == nil {
		l.err = err
	}
	if _, err := l.out.Write(line); err != nil && l.err == nil {
		l.err = err
	}
}

// flush writes a last line that had no newline.
func (l *lineWriter) flush() error {
	if len(l.buf) > 0 {
		l.emit(append(l.buf, '\n'))
		l.buf = nil
	}
	return l.err
}
