package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplayWaitsForNativeReadinessBeforeObservation(t *testing.T) {
	t.Setenv("JARVIS_DIAGNOSTIC_TRANSIENT_ATTACH", "1")
	log := filepath.Join(t.TempDir(), "ops")
	t.Setenv("JARVIS_DIAGNOSTIC_LOG", log)
	a, req := diagnosticApp(t, "Diagnostic bank")
	id, err := a.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	view, err := a.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.Admission.Attempts != 2 || view.Admission.LastTransientCode != "window_missing" || view.Observation == nil {
		t.Fatalf("replay did not recover native admission: %+v error=%s", view.Admission, view.Error)
	}
	ops, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ops), "attach\nattach\nobserve\n") || !strings.Contains(string(ops), "detach\n") {
		t.Fatalf("unexpected admission/cleanup sequence: %s", ops)
	}
}
