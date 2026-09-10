package app

import (
	"errors"

	"github.com/bharathvbcr/Manvi/manvi/workflow/catalog"
)

// RequireApprovedForUnattended refuses draft/revoked/missing catalog entries
// when replay runs with --unattended.
func (a *App) RequireApprovedForUnattended(capabilityPath string) error {
	p, err := a.Compile(capabilityPath)
	if err != nil {
		return err
	}
	entry, err := a.Catalog().FindByDigest(p.Digest())
	if err != nil {
		return errors.New("unattended replay refuses capability not approved in catalog")
	}
	return catalog.CheckUnattended(entry)
}
