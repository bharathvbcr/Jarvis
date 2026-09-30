package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDiscoverRejectsTaskOutsideByteBounds(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	for _, task := range []string{"", strings.Repeat("a", 8193)} {
		_, err = a.Discover(DiscoveryRequest{Tenant: "north", Task: task})
		if err == nil || err.Error() != "task must contain 1..8192 bytes" {
			t.Fatalf("task len %d: %v", len(task), err)
		}
	}
}

func TestCloseReportsRunWithNoExitSignal(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	a.runs["missing-exit"] = &runEntry{}
	if err = a.Close(); err == nil {
		t.Fatal("close succeeded while a run had no exit signal")
	}
}

func TestCloseUsesOneDeadlineForEveryLiveRun(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		a.runs[id] = &runEntry{done: make(chan struct{})}
	}
	started := time.Now()
	err = a.Close()
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("close succeeded while runs were still active")
	}
	if elapsed < 3*time.Second || elapsed >= 9*time.Second {
		t.Fatalf("close took %s; one shared 5s deadline should cover every live run", elapsed)
	}
}

func TestAdmissionAfterCancelDoesNotPublish(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		a, req := diagnosticApp(t, "Diagnostic bank")
		if err := admitDuringClose(t, a, func() error {
			_, err := a.Start(req)
			return err
		}); err == nil {
			t.Fatal("Start published a run after Close")
		}
	})
	t.Run("discover", func(t *testing.T) {
		a, _ := diagnosticApp(t, "Diagnostic bank")
		if err := admitDuringClose(t, a, func() error {
			_, err := a.Discover(DiscoveryRequest{
				Tenant:   "north",
				Task:     "read the balance",
				Provider: DiscoveryProviderFixture,
				PID:      1,
			})
			return err
		}); err == nil {
			t.Fatal("Discover published a run after Close")
		}
	})
}

func TestClosedAppRejectsConcurrentAdmission(t *testing.T) {
	a, req := diagnosticApp(t, "Diagnostic bank")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := a.Start(req)
			failures <- err
		}()
		go func() {
			defer wg.Done()
			_, err := a.Discover(DiscoveryRequest{
				Tenant:   "north",
				Task:     "read the balance",
				Provider: DiscoveryProviderFixture,
				PID:      1,
			})
			failures <- err
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err == nil {
			t.Fatal("closed app admitted a run")
		}
	}
}

func admitDuringClose(t *testing.T, a *App, admit func() error) error {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	previous := admitGate
	admitGate = func() {
		close(entered)
		<-release
	}
	t.Cleanup(func() { admitGate = previous })
	done := make(chan error, 1)
	go func() { done <- admit() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("admission did not reach the publish lock")
	}
	if err := a.Close(); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	return <-done
}
