package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// DriftReport is a read-only ladder dry-resolve across every capability target.
type DriftReport struct {
	Tenant       string                 `json:"tenant"`
	CapabilityID string                 `json:"capability_id"`
	Revision     string                 `json:"revision"`
	Digest       string                 `json:"digest"`
	Overlay      string                 `json:"overlay,omitempty"`
	WindowTitle  string                 `json:"window_title"`
	Targets      []computer.TargetDrift `json:"targets"`
}

// CompileForTenant compiles a capability and applies the profile overlay when
// the tenant binding references one. Digest covers base ‖ overlay when applied.
func (a *App) CompileForTenant(path, tenant string) (*workflow.Program, tenantBinding, error) {
	binding, err := a.profile(tenant)
	if err != nil {
		return nil, binding, err
	}
	raw, err := readBounded(a.resolve(path), workflow.MaxArtifactBytes)
	if err != nil {
		return nil, binding, err
	}
	// Identify the base first so the tenant's specialization for this exact
	// capability revision can be selected. Applying a tenant's only overlay to
	// whatever was being compiled meant a tenant that specialized one capability
	// could not run any other.
	base, err := workflow.Compile(raw)
	if err != nil {
		return nil, binding, err
	}
	p := base
	if selected, ok := binding.overlayFor(base.Capability().ID, base.Capability().Revision); ok {
		binding.OverlayPath = selected.Path
		binding.OverlayBytes = selected.Bytes
		overlay := selected.Overlay
		binding.Overlay = &overlay
		if p, err = workflow.CompileWithOverlay(raw, selected.Bytes); err != nil {
			return nil, binding, err
		}
	}
	if p.Capability().Application != "jarvis-bank" {
		return nil, binding, errors.New("this product profile only admits jarvis-bank capabilities")
	}
	if err = validateBankProgram(p, binding.Profile, binding.Policy); err != nil {
		return nil, binding, err
	}
	return p, binding, nil
}

// BuildDriftReport walks every target ladder against one observation.
func BuildDriftReport(p *workflow.Program, tenant, overlayPath, windowTitle string, obs computer.Observation) DriftReport {
	c := p.Capability()
	return DriftReport{
		Tenant:       tenant,
		CapabilityID: c.ID,
		Revision:     c.Revision,
		Digest:       p.Digest(),
		Overlay:      overlayPath,
		WindowTitle:  windowTitle,
		Targets:      computer.DryResolveTargets(c.Targets, obs),
	}
}

// Drift dry-resolves every target for a capability/tenant without acting.
// It launches or attaches the bank, observes once, reports rung index/ambiguity,
// then detaches. No Act calls are issued.
func (a *App) Drift(ctx context.Context, capabilityPath, tenant string, pid uint32) (DriftReport, error) {
	var out DriftReport
	if err := a.ctx.Err(); err != nil {
		return out, err
	}
	p, binding, err := a.CompileForTenant(capabilityPath, tenant)
	if err != nil {
		return out, err
	}
	client, err := a.broker()
	if err != nil {
		return out, err
	}
	runID, err := identifier()
	if err != nil {
		return out, err
	}
	var bank *ownedBank
	ownedPID := pid
	if ownedPID == 0 {
		bankDir := filepath.Join(a.cfg.Root, ".local", "bank", "drift-"+runID)
		if err = os.MkdirAll(bankDir, 0700); err != nil {
			return out, err
		}
		bank, err = startOwnedBank(ctx, a.cfg.Bank, "--tenant", tenant, "--state", filepath.Join(bankDir, "state.json"))
		if err != nil {
			return out, err
		}
		ownedPID = uint32(bank.cmd.Process.Pid)
		defer func() { _ = bank.stop() }()
	}
	attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	session, _, err := client.AttachFocusedReady(attachCtx, runID, ownedPID, binding.Profile.ReadOnlyTargets)
	cancel()
	if err != nil {
		err = bank.admissionError(err)
		return out, errors.Join(err, a.discardBroker(client))
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = client.Control(c, session, "detach")
	}()
	if session.Window.Title != binding.Profile.WindowTitle {
		return out, fmt.Errorf("attached window %q does not match profile", session.Window.Title)
	}
	// A freshly launched window is usually still settling, and replay handles
	// that with bounded re-observation. Observing once here made the diagnostic
	// fail where execution succeeds: every campaign run needed exactly one
	// retry, so drift reported observation_inconsistent almost every time. Reuse
	// the engine's own definition of a transient capture failure and its budget.
	attempts := p.Capability().Limits.ObservationAttempts
	if attempts < 1 {
		attempts = 1
	}
	var obs computer.Observation
	for attempt := 1; ; attempt++ {
		obsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		obs, err = client.Observe(obsCtx, session)
		cancel()
		if err == nil {
			break
		}
		if _, transient := computer.TransientObservation(err); !transient || attempt >= attempts {
			return out, err
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return BuildDriftReport(p, tenant, binding.OverlayPath, binding.Profile.WindowTitle, obs), nil
}
