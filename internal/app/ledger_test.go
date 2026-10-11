package app

import (
	"context"
	"errors"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/llm"
	"github.com/bharathvbcr/Manvi/manvi/llm/budget"
	"github.com/bharathvbcr/Manvi/manvi/llm/gemini"
	"github.com/bharathvbcr/Manvi/manvi/llm/transport"
)

func TestOpenCampaignLedgerUnifiesAssistAndDiscovery(t *testing.T) {
	root := t.TempDir()
	ledger, err := openCampaignLedger(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Limit() != campaignLedgerSoftCap || campaignLedgerSoftCap != 25_000_000_000 {
		t.Fatalf("soft cap drifted: limit=%d", ledger.Limit())
	}
	if campaignLedgerPrices.Input != 1500 || campaignLedgerPrices.Output != 7500 {
		t.Fatalf("prices=%+v", campaignLedgerPrices)
	}
	if err := campaignLedgerPrices.Validate(); err != nil {
		t.Fatalf("campaign prices cannot bound a charge: %v", err)
	}
	if campaignPriceRevision != "google-standard-2027-conservative-2026-09-09" {
		t.Fatalf("revision=%s", campaignPriceRevision)
	}
}

// The live campaign provider must be one budget.Provider will meter: it needs
// prices and the attempt gate, and a wrapper that hid either would make every
// live call fail closed.
func TestCampaignGeminiIsMeterable(t *testing.T) {
	var p llm.Provider = &campaignGemini{Adapter: gemini.New("", nil)}
	if pricer, ok := p.(budget.Pricer); !ok {
		t.Fatal("campaignGemini publishes no prices")
	} else if got, ok := pricer.PriceOf("any-model"); !ok || got != campaignLedgerPrices {
		t.Fatalf("PriceOf = %+v, %v", got, ok)
	}
	if gated, ok := p.(transport.AttemptGatedProvider); !ok || !gated.AttemptGateSupported() {
		t.Fatal("campaignGemini hides the attempt gate")
	}
	ledger, err := openCampaignLedger(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n, err := (&budget.Provider{Inner: p, Ledger: ledger}).Estimate(llm.Request{Model: DiscoveryModel}); err != nil || n <= 0 {
		t.Fatalf("the campaign model cannot be metered: reserve=%d err=%v", n, err)
	}
}

// A caller that has already given up must not get a ledger handle: Manvi's
// Open checks ctx before it takes the file lock, so a cancelled discovery or
// assist run stops here instead of opening, and later reserving against, the
// shared cap.
func TestOpenCampaignLedgerStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ledger, err := openCampaignLedger(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) || ledger != nil {
		t.Fatalf("ledger=%v err=%v, want nil and context.Canceled", ledger, err)
	}
}
