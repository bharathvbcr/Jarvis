package app

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOwnedBankSurvivesActionCancellationUntilReconciliationAndCleanup(t *testing.T) {
	t.Setenv("JARVIS_OWNED_BANK_FIXTURE", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, err := startOwnedBank(ctx, exe, "bank-fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer b.stop()
	cancel()
	select {
	case <-b.done:
		t.Fatal("action cancellation killed the bank before same-session reconciliation")
	case <-time.After(100 * time.Millisecond):
	}
	if err = b.stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-b.done:
	default:
		t.Fatal("cleanup returned before bank exit")
	}
	if _, err = startOwnedBank(ctx, exe, "bank-fixture"); err != context.Canceled {
		t.Fatalf("cancelled admission started a bank: %v", err)
	}
}
