package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// Profiles select a tenant/platform surface. Their policy can only narrow the
// reviewed application policy loaded from policies/<application>.json.
//
// A tenant runs many capabilities but an overlay specializes exactly one, which
// it names by capability_id and revision. Profiles therefore carry a set of
// overlays and each compile applies only the one naming that capability; a
// capability no overlay names compiles from the reviewed base. Overlay is the
// single-entry shorthand and is equivalent to a one-element Overlays.
type Profile struct {
	SchemaVersion   int                 `json:"schema_version"`
	Tenant          string              `json:"tenant"`
	Application     string              `json:"application"`
	Policy          string              `json:"policy"`
	Platforms       []string            `json:"platforms"`
	WindowTitle     string              `json:"window_title"`
	ReadOnlyTargets []computer.Selector `json:"read_only_targets"`
	Overlay         string              `json:"overlay,omitempty"`
	Overlays        []string            `json:"overlays,omitempty"`
}

// tenantOverlay is one reviewed specialization and the exact bytes it was read
// from, so evidence records what was applied rather than a re-serialization.
type tenantOverlay struct {
	Path    string
	Bytes   []byte
	Overlay workflow.Overlay
}

type tenantBinding struct {
	Profile      Profile
	Policy       PolicyDocument
	PolicyBytes  []byte
	PolicySHA256 string
	BindingBytes []byte
	// Overlays holds every reviewed specialization this tenant declares.
	Overlays []tenantOverlay
	// The remaining fields describe the overlay selected for one compile and
	// stay empty until compileForTenant chooses one.
	OverlayPath  string
	OverlayBytes []byte
	Overlay      *workflow.Overlay
}

// overlayFor returns the reviewed specialization naming this exact capability
// revision. Selection is by identity, never by position, so adding an overlay
// for one capability cannot change how another compiles.
func (b tenantBinding) overlayFor(id, revision string) (tenantOverlay, bool) {
	for _, o := range b.Overlays {
		if o.Overlay.CapabilityID == id && o.Overlay.Revision == revision {
			return o, true
		}
	}
	return tenantOverlay{}, false
}

func (a *App) profile(tenant string) (tenantBinding, error) {
	var out tenantBinding
	if tenant != "north" && tenant != "south" {
		return out, errors.New("unknown tenant binding")
	}
	raw, err := readBounded(filepath.Join(a.cfg.Root, "profiles", tenant+".json"), 1<<20)
	if err != nil {
		return out, err
	}
	var p Profile
	if err = workflow.DecodeStrict(raw, &p); err != nil {
		return out, err
	}
	if p.SchemaVersion != 1 || p.Tenant != tenant || p.Application == "" || p.WindowTitle == "" {
		return out, errors.New("profile identity mismatch")
	}
	if p.Policy == "" {
		p.Policy = p.Application
	}
	if p.Policy != p.Application {
		return out, errors.New("profile policy reference must match application")
	}
	supported := false
	for _, platform := range p.Platforms {
		if platform == runtime.GOOS {
			supported = true
		}
	}
	if !supported {
		return out, errors.New("profile does not support this platform")
	}
	policy, policyBytes, policySHA, err := a.loadPolicy(p.Policy)
	if err != nil {
		return out, err
	}
	if policy.Application != p.Application {
		return out, errors.New("profile application does not match reviewed policy")
	}
	if !policy.AllowsWindow(p.WindowTitle) {
		return out, fmt.Errorf("profile window %q is outside reviewed policy", p.WindowTitle)
	}
	for _, target := range p.ReadOnlyTargets {
		if !policy.AllowsReadOnly(target) {
			return out, fmt.Errorf("binding attempts to widen input policy for %s", target.Name)
		}
	}
	out = tenantBinding{Profile: p, Policy: policy, PolicyBytes: policyBytes, PolicySHA256: policySHA, BindingBytes: raw}
	paths := p.Overlays
	if p.Overlay != "" {
		paths = append([]string{p.Overlay}, paths...)
	}
	for _, path := range paths {
		overlayRaw, err := readBounded(a.resolve(path), workflow.MaxArtifactBytes)
		if err != nil {
			return out, err
		}
		o, err := workflow.DecodeOverlay(overlayRaw)
		if err != nil {
			return out, err
		}
		if o.Tenant != p.Tenant {
			return out, errors.New("overlay tenant does not match profile")
		}
		if err = assertOverlayWithinPolicy(o, policy); err != nil {
			return out, err
		}
		// Two overlays naming one capability revision would make the applied
		// specialization depend on ordering. Refuse rather than pick.
		if _, duplicate := out.overlayFor(o.CapabilityID, o.Revision); duplicate {
			return out, fmt.Errorf("tenant declares two overlays for %s revision %s", o.CapabilityID, o.Revision)
		}
		out.Overlays = append(out.Overlays, tenantOverlay{Path: path, Bytes: overlayRaw, Overlay: o})
	}
	return out, nil
}

// assertOverlayWithinPolicy refuses overlay target rungs outside the reviewed
// allowlists. Overlays may only replace ladders with already-permitted selectors.
func assertOverlayWithinPolicy(o workflow.Overlay, policy PolicyDocument) error {
	for name, sel := range o.Targets {
		for i, rung := range sel.Ladder() {
			want := policySelectorFromWorkflow(rung)
			if policy.containsSelector(policy.ReadOnlyTargets, want) || policy.containsSelector(policy.EditableFields, want) {
				continue
			}
			return fmt.Errorf("overlay target %s rung %d widens policy with %s", name, i, want.label())
		}
	}
	return nil
}

func validateBankProgram(p *workflow.Program, profile Profile, policy PolicyDocument) error {
	for _, step := range p.Capability().Steps {
		if step.Kind == "conclude" {
			if !policy.AllowsStepKind(step.Kind) {
				return fmt.Errorf("step %s uses kind %q outside reviewed policy", step.ID, step.Kind)
			}
			continue
		}
		if !policy.AllowsStepKind(step.Kind) {
			return fmt.Errorf("step %s uses kind %q outside reviewed policy", step.ID, step.Kind)
		}
		if !policy.AllowsEffect(step.Effect) {
			return fmt.Errorf("step %s uses effect %q outside reviewed policy", step.ID, step.Effect)
		}
		if step.Effect == "change" && !policy.UnattendedChange {
			// Change-effect steps remain expressible; unattended_change=false means
			// the host must not treat them as silently auto-approved. Admission of
			// assisted/human control is enforced by the runner, not by deleting the kind.
		}
		selector := p.Capability().Targets[step.Target]
		if step.Kind == "extract" || step.Kind == "assert" || step.Kind == "branch" || step.Kind == "wait" {
			continue
		}
		if step.Effect == "change" {
			continue
		}
		if step.Kind == "set_value" || step.Kind == "type_text" {
			if !policyAllowsEditableLadder(policy, selector) {
				return errors.New("binding permits value entry only in reviewed editable fields")
			}
		}
		if !profileAllowsReadLadder(profile, selector) {
			return fmt.Errorf("read-effect step %s lacks a trusted profile binding", step.ID)
		}
	}
	return nil
}

func policyAllowsEditableLadder(policy PolicyDocument, selector workflow.Selector) bool {
	for _, rung := range selector.Ladder() {
		if !policy.AllowsEditable(rung) {
			return false
		}
	}
	return len(selector.Ladder()) > 0
}

func profileAllowsReadLadder(profile Profile, selector workflow.Selector) bool {
	for _, rung := range selector.Ladder() {
		want := policySelectorFromWorkflow(rung)
		found := false
		for _, allowed := range profile.ReadOnlyTargets {
			if policySelectorFromComputer(allowed).equal(want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return len(selector.Ladder()) > 0
}

// The trusted application boundary owns privacy. Capability metadata can ask
// for additional protection; it cannot make bank entry fields public.
func bankPrivacy(policy PolicyDocument, inputs map[string]workflow.Value) computer.PrivacyPolicy {
	p := computer.PrivacyPolicy{SensitiveTargets: policy.SensitiveComputerSelectors()}
	for _, v := range inputs {
		if v.Type == "string" && v.Text != "" {
			p.Watched = append(p.Watched, v.Text)
		}
	}
	return p
}
