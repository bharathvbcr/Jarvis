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
	entry, err := findByDigest(a.Catalog(), p.Digest())
	if err != nil {
		return errors.New("unattended replay refuses capability not approved in catalog")
	}
	return catalog.CheckUnattended(entry)
}

// findByDigest returns the catalog entry whose immutable digest matches.
func findByDigest(s catalog.Store, sha string) (catalog.Entry, error) {
	if sha == "" {
		return catalog.Entry{}, errors.New("digest required")
	}
	rows, err := s.List()
	if err != nil {
		return catalog.Entry{}, err
	}
	for _, e := range rows {
		if e.SHA256 == sha {
			return e, nil
		}
	}
	return catalog.Entry{}, errors.New("unknown catalog digest")
}
