package app

import (
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/llm"
	"github.com/bharathvbcr/Manvi/manvi/llm/budget"
	"github.com/bharathvbcr/Manvi/manvi/llm/gemini"
	"github.com/bharathvbcr/Manvi/manvi/llm/transport"
)

func TestOpenCampaignLedgerUnifiesAssistAndDiscovery(t *testing.T) {
	root := t.TempDir()
	ledger, err := openCampaignLedger(root)
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
	ledger, err := openCampaignLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n, err := (&budget.Provider{Inner: p, Ledger: ledger}).Estimate(llm.Request{Model: DiscoveryModel}); err != nil || n <= 0 {
		t.Fatalf("the campaign model cannot be metered: reserve=%d err=%v", n, err)
	}
}
