package app

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// The fixtures and counters here measure what one replay step costs the
// evidence journal and the run view: flushes issued, bytes written, and what a
// poll or a record copy allocates. A frame is sized like a real window capture
// (a few hundred KB to 2 MB of base64), because the cost being measured scales
// with the image, not with the tree.

// costFrame returns a deterministic pseudo-random "PNG" of n bytes as base64.
// The journal never decodes the image, so incompressible bytes of the right
// length are as good as a real capture. seed picks the content.
func costFrame(n int, seed int64) string {
	raw := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}

func costNodes(n int) []computer.Node {
	nodes := make([]computer.Node, n)
	for i := range nodes {
		value, identifier, parent := fmt.Sprintf("value-%d", i), fmt.Sprintf("id-%d", i), "root"
		bounds := computer.Bounds{X: float64(i), Y: float64(2 * i), Width: 120, Height: 24}
		nodes[i] = computer.Node{
			ID: fmt.Sprintf("node-%d", i), Role: "text_field", Name: fmt.Sprintf("Field %d", i),
			Value: &value, Identifier: &identifier, Parent: &parent, Bounds: &bounds,
			Enabled: true, Editable: i%2 == 0, Actions: []string{"press", "focus"}, NativePath: []uint32{0, uint32(i)},
		}
	}
	return nodes
}

func costObservation(id, frame string, nodes int) *computer.Observation {
	return &computer.Observation{
		ID: id, Epoch: 1, CapturedAtMillis: 1000, TreeAtMillis: 900, Complete: true,
		Window:     computer.Window{PID: 42, ID: 7, Title: "Bank", ProcessIdentity: "bank", Foreground: true, Scale: 2},
		Nodes:      costNodes(nodes),
		Screenshot: computer.Screenshot{MIMEType: "image/png", Base64: frame, Width: 1600, Height: 1200},
	}
}

// costReadStep is the record sequence of one read-effect replay step as the
// run loop emits it: the pre-dispatch observation, the dispatch admission, the
// receipt, and the post-dispatch observation, each with its reducer event.
func costReadStep(step int, frameA, frameB string) []computer.Record {
	id := fmt.Sprintf("run:step-%d", step)
	return []computer.Record{
		{Kind: "observation", Stage: "observe", ActionID: id, Actor: "automation", Epoch: 1, Observation: costObservation(fmt.Sprintf("obs-%d-a", step), frameA, 20)},
		{Kind: "event", ActionID: id, Actor: "automation", Epoch: 1, Event: &workflow.Event{Kind: "observed", ActionID: id, Epoch: 1}},
		{Kind: "action_started", ActionID: id, Actor: "automation", Epoch: 1},
		{Kind: "event", ActionID: id, Actor: "automation", Epoch: 1, Event: &workflow.Event{Kind: "receipt", ActionID: id, Epoch: 1, Delivery: "sent"}},
		{Kind: "observation", Stage: "observe_after", ActionID: id, Actor: "automation", Epoch: 1, Observation: costObservation(fmt.Sprintf("obs-%d-b", step), frameB, 20)},
		{Kind: "event", ActionID: id, Actor: "automation", Epoch: 1, Event: &workflow.Event{Kind: "observed", ActionID: id, Epoch: 1}},
	}
}

// flushLog counts and orders the flushes a journal issues.
type flushLog struct {
	mu    sync.Mutex
	files []string
}

func (l *flushLog) sync(f *os.File) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.files = append(l.files, filepath.Base(f.Name()))
	return nil
}
func (l *flushLog) count() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.files) }

// countedJournal opens a journal whose flushes are counted instead of issued.
func countedJournal(tb testing.TB) (*desktopJournal, *flushLog) {
	tb.Helper()
	j := openTestJournal(tb, filepath.Join(tb.TempDir(), "run"))
	log := &flushLog{}
	j.sync = log.sync
	return j, log
}

func openTestJournal(tb testing.TB, dir string) *desktopJournal {
	tb.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		tb.Fatal(err)
	}
	j, err := openDesktopJournal(dir)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := j.file.Close(); err != nil {
			tb.Errorf("closing the test journal: %v", err)
		}
	})
	return j
}

// TestJournalFlushesPerReadStep pins today's cost of one read-effect step with
// two distinct frames: ten flushes (two observations at three each, four other
// records at one each), and the bytes they write. It characterizes the
// behaviour the evidence-durability decision is about; a change to the count is
// a durability change and must be a deliberate one.
func TestJournalFlushesPerReadStep(t *testing.T) {
	j, log := countedJournal(t)
	frameA, frameB := costFrame(1<<20, 1), costFrame(1<<20, 2)
	var artifactBytes uint64
	for i, record := range costReadStep(1, frameA, frameB) {
		before := log.count()
		_, artifacts, err := j.append(record, uint64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range artifacts {
			artifactBytes += a.SizeBytes
		}
		want := 1
		if record.Kind == "observation" {
			want = 3
		}
		if got := log.count() - before; got != want {
			t.Errorf("%s/%s record issued %d flushes, want %d", record.Kind, record.Stage, got, want)
		}
	}
	if got := log.count(); got != 10 {
		t.Errorf("read step issued %d flushes, want 10", got)
	}
	written := int64(artifactBytes) + j.journalBytes
	t.Logf("read step: %d flushes, %d bytes written (%d frame+tree, %d journal lines)", log.count(), written, artifactBytes, j.journalBytes)
	if j.imageBytes != 2<<20 {
		t.Errorf("journal counted %d image bytes, want %d", j.imageBytes, 2<<20)
	}
}

// TestActionStartedIsFlushedBeforeAppendReturns is the safety property the
// evidence journal exists for: when append returns for an action_started
// record, its line is written and the file has been flushed after the write.
func TestActionStartedIsFlushedBeforeAppendReturns(t *testing.T) {
	j, _ := countedJournal(t)
	var sizeAtFlush int64
	j.sync = func(f *os.File) error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		sizeAtFlush = info.Size()
		return nil
	}
	if _, _, err := j.append(computer.Record{Kind: "action_started", ActionID: "run:step-1", Actor: "automation", Epoch: 1}, 1); err != nil {
		t.Fatal(err)
	}
	if sizeAtFlush == 0 || sizeAtFlush != j.journalBytes {
		t.Fatalf("flush saw %d bytes of the line, journal wrote %d: the line was not written before the flush", sizeAtFlush, j.journalBytes)
	}
}

func BenchmarkJournalAppendReadStep(b *testing.B) {
	for _, mode := range []string{"counted", "fsync"} {
		b.Run(mode, func(b *testing.B) {
			frameA, frameB := costFrame(1<<20, 1), costFrame(1<<20, 2)
			var log *flushLog
			var j *desktopJournal
			if mode == "counted" {
				j, log = countedJournal(b)
			} else {
				j = openTestJournal(b, filepath.Join(b.TempDir(), "run"))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				j.records = 0 // each op is one step; the record limit is not what is measured
				for k, record := range costReadStep(i, frameA, frameB) {
					if _, _, err := j.append(record, uint64(i*6+k+1)); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.StopTimer()
			if log != nil {
				b.ReportMetric(float64(log.count())/float64(b.N), "flushes/step")
			}
			b.ReportMetric(float64(j.imageBytes)/float64(b.N), "frameB/step")
			b.ReportMetric(float64(j.journalBytes)/float64(b.N), "journalB/step")
		})
	}
}

// costEntry is a run entry holding records accumulated records and a last
// observation carrying a frame of frameBytes bytes.
func costEntry(tb testing.TB, records, frameBytes int) (*App, *runEntry) {
	tb.Helper()
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		tb.Fatal(err)
	}
	p, err := workflow.Compile(raw)
	if err != nil {
		tb.Fatal(err)
	}
	e := &runEntry{program: p, done: make(chan struct{})}
	e.view.RunID = "run"
	e.view.Capability = p.Capability()
	for i := 0; i < records; i++ {
		switch i % 3 {
		case 0:
			r := computer.Record{Kind: "observation", Stage: "observe", ActionID: "run:step", Actor: "automation", Epoch: 1, Observation: costObservation(fmt.Sprintf("obs-%d", i), "", 20)}
			e.view.Records = append(e.view.Records, r)
		case 1:
			e.view.Records = append(e.view.Records, computer.Record{Kind: "event", ActionID: "run:step", Event: &workflow.Event{Kind: "observed", Epoch: 1}})
		default:
			e.view.Records = append(e.view.Records, computer.Record{Kind: "action_started", ActionID: "run:step", Receipt: &computer.Receipt{Delivery: "sent"}})
		}
	}
	e.view.Observation = costObservation("last", costFrame(frameBytes, 9), 20)
	a := &App{runs: map[string]*runEntry{"run": e}}
	return a, e
}

func BenchmarkRunGet(b *testing.B) {
	for _, records := range []int{10, 100, 1000} {
		for _, frame := range []int{80 << 10, 1500 << 10} {
			b.Run(fmt.Sprintf("records=%d/frame=%dKB", records, frame>>10), func(b *testing.B) {
				a, _ := costEntry(b, records, frame)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := a.Get("run"); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkOnRecordCopy(b *testing.B) {
	for _, frame := range []int{80 << 10, 1500 << 10} {
		b.Run(fmt.Sprintf("frame=%dKB", frame>>10), func(b *testing.B) {
			_, e := costEntry(b, 0, 0)
			record := computer.Record{Kind: "observation", Stage: "observe", ActionID: "run:step", Actor: "automation", Epoch: 1, Observation: costObservation("obs", costFrame(frame, 3), 20)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := e.appendRecord(record); err != nil {
					b.Fatal(err)
				}
				e.view.Records = e.view.Records[:0]
			}
		})
	}
}

// allocatedBytes is the average heap allocation of f over runs calls.
func allocatedBytes(runs int, f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		f()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

// TestRunPollAndRecordCostIsRecorded logs today's per-poll and per-record
// allocation so the numbers sit next to the benchmark that produced them.
func TestRunPollAndRecordCostIsRecorded(t *testing.T) {
	a, e := costEntry(t, 100, 1500<<10)
	poll := allocatedBytes(20, func() {
		if _, err := a.Get("run"); err != nil {
			t.Fatal(err)
		}
	})
	record := computer.Record{Kind: "observation", Stage: "observe", ActionID: "run:step", Actor: "automation", Epoch: 1, Observation: costObservation("obs", costFrame(1500<<10, 3), 20)}
	copied := allocatedBytes(20, func() {
		if err := e.appendRecord(record); err != nil {
			t.Fatal(err)
		}
		e.view.Records = e.view.Records[:100]
	})
	t.Logf("one Get() poll of 100 records with a 1.5 MB frame allocates %d bytes; one onRecord copy of a 1.5 MB-frame record allocates %d bytes", poll, copied)
}
