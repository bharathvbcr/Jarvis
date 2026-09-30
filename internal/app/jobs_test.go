package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/serve"
)

func awaitJob(t *testing.T, registry *jobRegistry, id string) JobView {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		v, err := registry.get(id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Status != "pending" && v.Status != "running" {
			return v
		}
		select {
		case <-deadline.C:
			t.Fatal("background job did not finish")
		case <-tick.C:
		}
	}
}

func TestJobsOwnResultsAndPreserveOperation(t *testing.T) {
	var registry jobRegistry
	result := json.RawMessage(`{"answer":42}`)
	initial, err := registry.start(context.Background(), "jarvis.test", time.Second, func(context.Context) (json.RawMessage, error) { return result, nil })
	if err != nil {
		t.Fatal(err)
	}
	if initial.Operation != "jarvis.test" || initial.Status != "pending" || len(initial.JobID) != 32 || initial.Result != nil {
		t.Fatalf("invalid admission: %+v", initial)
	}
	finished := awaitJob(t, &registry, initial.JobID)
	if finished.Status != "succeeded" || string(finished.Result) != `{"answer":42}` || finished.Operation != initial.Operation {
		t.Fatalf("invalid completion: %+v", finished)
	}
	result[0], finished.Result[1] = '[', 'X'
	again, err := registry.get(initial.JobID)
	if err != nil || string(again.Result) != `{"answer":42}` {
		t.Fatalf("mutable job result: %+v %v", again, err)
	}
}

func TestJobsBoundPhysicalWorkersAfterDeadline(t *testing.T) {
	var registry jobRegistry
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	jobs := make([]JobView, 0, maxJobs)
	for range maxJobs {
		job, err := registry.start(parent, "blocked", 5*time.Millisecond, func(context.Context) (json.RawMessage, error) { <-release; return json.RawMessage(`{}`), nil })
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	for _, job := range jobs {
		v := awaitJob(t, &registry, job.JobID)
		if v.Status != "failed" || !strings.Contains(v.Error, "deadline") || len(v.Result) != 0 {
			t.Fatalf("deadline falsely succeeded: %+v", v)
		}
	}
	if _, err := registry.start(parent, "seventeenth", time.Second, func(context.Context) (json.RawMessage, error) {
		t.Error("capacity-rejected work executed")
		return nil, nil
	}); err == nil {
		t.Fatal("expired but physically blocked workers released their admission slots")
	}
}

func TestJobsCancellationFailureAndInvalidResults(t *testing.T) {
	cases := []struct {
		name string
		work func(context.Context) (json.RawMessage, error)
	}{
		{"failure", func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(`{"partial":true}`), errors.New("worker refused")
		}},
		{"panic", func(context.Context) (json.RawMessage, error) { panic("private panic details") }},
		{"invalid", func(context.Context) (json.RawMessage, error) { return json.RawMessage(`not JSON`), nil }},
		{"oversized", func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(`"` + strings.Repeat("a", maxJobResultBytes) + `"`), nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var registry jobRegistry
			job, err := registry.start(context.Background(), tc.name, time.Second, tc.work)
			if err != nil {
				t.Fatal(err)
			}
			v := awaitJob(t, &registry, job.JobID)
			if v.Status != "failed" || v.Error == "" || len(v.Result) != 0 || strings.Contains(v.Error, "private panic") {
				t.Fatalf("failure leaked result or panic: %+v", v)
			}
		})
	}
	var registry jobRegistry
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	job, err := registry.start(parent, "cancelled", time.Second, func(ctx context.Context) (json.RawMessage, error) {
		close(started)
		<-ctx.Done()
		return json.RawMessage(`{"false_success":true}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	v := awaitJob(t, &registry, job.JobID)
	if v.Status != "cancelled" || v.Error == "" || len(v.Result) != 0 {
		t.Fatalf("cancelled work succeeded: %+v", v)
	}
	if _, err := registry.start(parent, "after-close", time.Second, func(context.Context) (json.RawMessage, error) {
		t.Error("work admitted after parent cancellation")
		return nil, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission: %v", err)
	}
}

func TestJobsExpireAndEvictOnlyCompletedResults(t *testing.T) {
	var registry jobRegistry
	var first JobView
	for i := 0; i <= maxJobs; i++ {
		job, err := registry.start(context.Background(), "small", time.Second, func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{}`), nil })
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = job
		}
		awaitJob(t, &registry, job.JobID)
	}
	if _, err := registry.get(first.JobID); err == nil {
		t.Fatal("oldest completed job was not evicted")
	}
	registry.mu.Lock()
	if len(registry.jobs) != maxJobs || registry.retained != maxJobs*2 {
		t.Errorf("invalid retention accounting: %d jobs, %d bytes", len(registry.jobs), registry.retained)
	}
	for _, job := range registry.jobs {
		job.finished = time.Now().Add(-jobRetention)
	}
	registry.prune(time.Now())
	if len(registry.jobs) != 0 || registry.retained != 0 {
		t.Error("expired results remain retained")
	}
	registry.mu.Unlock()
}

func TestJobsBoundRetainedResultBytes(t *testing.T) {
	var registry jobRegistry
	result := append([]byte(`"`), bytes.Repeat([]byte("a"), maxJobResultBytes-2)...)
	result = append(result, '"')
	var first JobView
	for i := 0; i < 5; i++ {
		job, err := registry.start(context.Background(), "large", 3*time.Second, func(context.Context) (json.RawMessage, error) { return result, nil })
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = job
		}
		if v := awaitJob(t, &registry, job.JobID); v.Status != "succeeded" {
			t.Fatalf("bounded valid result failed: %s", v.Error)
		}
	}
	if _, err := registry.get(first.JobID); err == nil {
		t.Fatal("result retention exceeded 64 MiB without eviction")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.retained != maxRetainedJobBytes || len(registry.jobs) != 4 {
		t.Fatalf("bad byte accounting: %d bytes across %d jobs", registry.retained, len(registry.jobs))
	}
}

func TestExportPublishesCompleteArchiveAndPreservesExistingDestination(t *testing.T) {
	a, _ := diagnosticApp(t, "Diagnostic bank")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "public.json"), []byte(`{"public":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.runs["complete"] = &runEntry{view: RunView{RunID: "complete", Finished: true, EvidenceDir: dir}}
	destination := filepath.Join(t.TempDir(), "evidence.zip")
	path, err := a.Export("complete", destination)
	if err != nil || path != destination {
		t.Fatalf("export: %q %v", path, err)
	}
	original, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil || len(z.File) != 1 || z.File[0].Name != "public.json" {
		t.Fatalf("invalid completed archive: %v", err)
	}
	if _, err := a.Export("complete", destination); err == nil {
		t.Fatal("existing destination replaced")
	}
	again, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(original, again) {
		t.Fatal("existing archive changed")
	}
	if err := os.Symlink(filepath.Join(dir, "public.json"), filepath.Join(dir, "z-symlink")); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(filepath.Dir(destination), "failed.zip")
	if _, err := a.Export("complete", failed); err == nil {
		t.Fatal("nonregular evidence exported")
	}
	if _, err := os.Stat(failed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial export destination survived: %v", err)
	}
	staging, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".jarvis-export-*"))
	if err != nil || len(staging) != 0 {
		t.Fatalf("export staging files survived: %v %v", staging, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.export(ctx, "complete", failed); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled export: %v", err)
	}
}

func TestSlowDoctorDoesNotBlockHostStatusOrCancellation(t *testing.T) {
	t.Setenv("JARVIS_DIAGNOSTIC_PROBE_DELAY", "1")
	a, _ := diagnosticApp(t, "Diagnostic bank")
	cancelled := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(cancelled)
			close(done)
		})
	}
	a.mu.Lock()
	a.runs["waiting"] = &runEntry{done: done, cancel: finish, view: RunView{RunID: "waiting"}}
	a.mu.Unlock()
	t.Cleanup(finish)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer input.Close()
	defer send.Close()
	defer output.Close()
	defer receive.Close()
	server := serve.New(receive, serve.Options{HardRules: true, Modules: []serve.Module{a}})
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, input) }()
	replies := make(chan serve.Response, 8)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var response serve.Response
			if decoder.Decode(&response) != nil {
				return
			}
			replies <- response
		}
	}()
	sent := make(chan error, 1)
	go func() {
		enc := json.NewEncoder(send)
		for _, request := range []serve.Request{{ID: "hello", Op: "hello", Params: json.RawMessage(`{"protocol":1,"host":"job-test"}`)}, {ID: "doctor", Op: "jarvis.doctor", Params: json.RawMessage(`{}`)}, {ID: "get", Op: "jarvis.run.get", Params: json.RawMessage(`{"run_id":"waiting"}`)}, {ID: "cancel", Op: "jarvis.run.control", Params: json.RawMessage(`{"run_id":"waiting","control":{"kind":"cancel"}}`)}} {
			if err := enc.Encode(request); err != nil {
				sent <- err
				return
			}
		}
		sent <- nil
	}()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	var hello, admitted, got bool
	for {
		select {
		case response := <-replies:
			if !response.OK {
				t.Fatalf("host response failed: %+v", response)
			}
			switch response.ID {
			case "hello":
				var h serve.HelloResult
				if err := json.Unmarshal(response.Result, &h); err != nil {
					t.Fatal(err)
				}
				for _, op := range h.Ops {
					if op == "jarvis.job.get" {
						hello = true
					}
				}
			case "doctor":
				var job JobView
				if err := json.Unmarshal(response.Result, &job); err != nil {
					t.Fatal(err)
				}
				admitted = job.Status == "pending" && job.Operation == "jarvis.doctor" && len(job.JobID) == 32 && len(job.Result) == 0
			case "get":
				got = true
			}
			if response.ID == "cancel" {
				if !hello || !admitted || !got {
					t.Fatalf("incomplete job protocol: hello=%v admission=%v get=%v", hello, admitted, got)
				}
				select {
				case <-cancelled:
				case <-time.After(time.Second):
					t.Fatal("cancel callback not delivered")
				}
				return
			}
		case <-timer.C:
			t.Fatal("slow doctor blocked run.get/cancel on the host wire")
		}
	}
}
