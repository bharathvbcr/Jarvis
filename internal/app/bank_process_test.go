package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
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

func TestOwnedBankNilSafety(t *testing.T) {
	var nilBank *ownedBank
	if err := nilBank.stop(); err != nil {
		t.Fatalf("nilBank.stop() returned error: %v", err)
	}
	baseErr := context.Canceled
	if got := nilBank.admissionError(baseErr); got != baseErr {
		t.Fatalf("nilBank.admissionError() = %v; want %v", got, baseErr)
	}

	emptyBank := &ownedBank{}
	if err := emptyBank.stop(); err != nil {
		t.Fatalf("emptyBank.stop() returned error: %v", err)
	}
	if got := emptyBank.admissionError(baseErr); got != baseErr {
		t.Fatalf("emptyBank.admissionError() = %v; want %v", got, baseErr)
	}
}

func TestOwnedBankStopKillsProcessWithoutExitSignal(t *testing.T) {
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
	t.Cleanup(func() {
		if b.cmd != nil && b.cmd.Process != nil {
			_ = b.cmd.Process.Kill()
		}
		select {
		case <-b.done:
		case <-time.After(2 * time.Second):
		}
	})
	// Same live process, but no exit signal. stop must still kill it and must
	// not report that cleanup succeeded.
	orphan := &ownedBank{cmd: b.cmd}
	if err = orphan.stop(); err == nil {
		t.Fatal("stop reported success without an exit signal")
	}
	select {
	case <-b.done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop left the bank process running")
	}
}

func TestOwnedBankStopAndAdmissionRace(t *testing.T) {
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
	var wg sync.WaitGroup
	wg.Add(8)
	for i := 0; i < 4; i++ {
		go func() {
			defer wg.Done()
			_ = b.admissionError(errors.New("attach"))
		}()
		go func() {
			defer wg.Done()
			_ = b.stop()
		}()
	}
	wg.Wait()
	select {
	case <-b.done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent stop did not reap the bank")
	}
}

func TestOwnedBankCleanupStress(t *testing.T) {
	t.Setenv("JARVIS_OWNED_BANK_FIXTURE", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		b, err := startOwnedBank(ctx, exe, "bank-fixture")
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		got := b.admissionError(errors.New("attach"))
		if got == nil || !strings.Contains(got.Error(), "attach") {
			cancel()
			b.stop()
			t.Fatalf("admission %d: %v", i, got)
		}
		if err = b.stop(); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
	}
}
