package app

import (
	"path/filepath"

	"github.com/bharathvbcr/Manvi/manvi/llm/budget"
)

// campaignLedgerPrices are the conservative Gemini standard rates used for
// assisted recovery and discovery campaign accounting.
var campaignLedgerPrices = budget.Prices{
	InputNanoUSD:    1500,
	OutputNanoUSD:   7500,
	MaxInputTokens:  1048576,
	MaxOutputTokens: 65536,
	Revision:        "google-standard-2027-conservative-2026-09-09",
}

const campaignLedgerSoftCap = 25_000_000_000

func openCampaignLedger(root string) (*budget.Ledger, error) {
	return budget.Open(filepath.Join(root, ".local", "gemini-campaign.json"), campaignLedgerSoftCap, campaignLedgerPrices)
}
