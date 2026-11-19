package app

import (
	"errors"
	"fmt"

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
	if err := catalog.CheckUnattended(entry); err != nil {
		return err
	}
	return a.refuseAgentPromotion(entry)
}

// refuseAgentPromotion refuses a revision whose latest promotion an agent made
// in yolo mode. The catalog's status says only "approved", so without this an
// agent could promote a capability (one it registered itself, even) and a later
// unattended replay, started by anyone in default mode, would run it as if a
// person had approved it. A person promoting the same revision again appends a
// record of their own, which is then the latest. A revision with no record is
// left to the catalog status, as before; a history that cannot be read refuses.
func (a *App) refuseAgentPromotion(entry catalog.Entry) error {
	history, err := a.history(entry.ID)
	if err != nil {
		return fmt.Errorf("unattended replay could not read the promotion history of %s: %w", entry.ID, err)
	}
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		if h.Revision != entry.Revision || h.SHA256 != entry.SHA256 {
			continue
		}
		if h.PromotedBy != "" {
			return fmt.Errorf("unattended replay refuses %s revision %s: it was promoted by %s in yolo mode, and a person must promote it before it runs unattended", entry.ID, entry.Revision, h.PromotedBy)
		}
		return nil
	}
	return nil
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
