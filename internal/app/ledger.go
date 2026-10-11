package app

import (
	"context"
	"path/filepath"

	"github.com/bharathvbcr/Manvi/manvi/llm/budget"
	"github.com/bharathvbcr/Manvi/manvi/llm/gemini"
)

// campaignPriceRevision names the rate table below in the discovery record.
const campaignPriceRevision = "google-standard-2027-conservative-2026-09-09"

// campaignLedgerPrices are the conservative Gemini standard rates used for
// assisted recovery and discovery campaign accounting, in nano-USD per token.
// Cached input is charged at the full input rate and reasoning is added on top
// of output, so a charge can only be over-stated.
var campaignLedgerPrices = budget.Prices{Rates: budget.Rates{
	Input:      1500,
	Output:     7500,
	CacheWrite: 1500,
	CacheRead:  1500,
}}

const campaignLedgerSoftCap = 25_000_000_000

// campaignGemini is the Gemini adapter with the campaign's price table. Manvi's
// budget.Provider takes prices from the provider it meters, and the adapter
// publishes none.
type campaignGemini struct{ *gemini.Adapter }

// PriceOf answers budget.Pricer for every model the campaign may use.
func (*campaignGemini) PriceOf(string) (budget.Prices, bool) { return campaignLedgerPrices, true }

// openCampaignLedger opens the campaign ledger, waiting for its file lock no
// longer than ctx allows.
func openCampaignLedger(ctx context.Context, root string) (*budget.Ledger, error) {
	return budget.Open(ctx, filepath.Join(root, ".local", "gemini-campaign.json"), campaignLedgerSoftCap)
}
