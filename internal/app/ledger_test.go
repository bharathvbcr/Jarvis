package app

import (
	"testing"
)

func TestOpenCampaignLedgerUnifiesAssistAndDiscovery(t *testing.T) {
	root := t.TempDir()
	ledger, err := openCampaignLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	if campaignLedgerSoftCap != 25_000_000_000 {
		t.Fatal("soft cap drifted")
	}
	if campaignLedgerPrices.InputNanoUSD != 1500 || campaignLedgerPrices.OutputNanoUSD != 7500 {
		t.Fatalf("prices=%+v", campaignLedgerPrices)
	}
	if campaignLedgerPrices.Revision != "google-standard-2027-conservative-2026-09-09" {
		t.Fatalf("revision=%s", campaignLedgerPrices.Revision)
	}
}
