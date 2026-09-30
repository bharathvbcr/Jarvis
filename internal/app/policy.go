package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// PolicySelector is the reviewed allowlist shape for UI targets. It mirrors the
// current Selector fields (role/name/identifier/ancestor/visual) so policy
// documents stay tolerant while Manvi evolves workflow schema in parallel.
type PolicySelector struct {
	Visual     *workflow.VisualAnchor `json:"visual,omitempty"`
	Role       string                 `json:"role,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Identifier string                 `json:"identifier,omitempty"`
	Ancestor   string                 `json:"ancestor,omitempty"`
}

// PolicyDocument is the reviewed application allowlist. Profiles and overlays
// may only narrow it — never widen windows, targets, kinds, effects, or the
// unattended_change flag.
type PolicyDocument struct {
	SchemaVersion      int              `json:"schema_version"`
	Application        string           `json:"application"`
	Windows            []string         `json:"windows"`
	EditableFields     []PolicySelector `json:"editable_fields"`
	ReadOnlyTargets    []PolicySelector `json:"read_only_targets"`
	PermittedStepKinds []string         `json:"permitted_step_kinds"`
	PermittedEffects   []string         `json:"permitted_effects"`
	SensitiveTargets   []PolicySelector `json:"sensitive_targets"`
	UnattendedChange   bool             `json:"unattended_change"`
}

// PolicySHA256 returns the SHA-256 hex digest of the reviewed policy bytes.
// Callers must hash the exact on-disk document admitted into a run.
func PolicySHA256(raw []byte) string { return digest(raw) }

func (a *App) loadPolicy(application string) (PolicyDocument, []byte, string, error) {
	var p PolicyDocument
	if application == "" || strings.Contains(application, "/") || strings.Contains(application, `\`) || strings.Contains(application, "..") {
		return p, nil, "", errors.New("invalid policy application id")
	}
	raw, err := readBounded(filepath.Join(a.cfg.Root, "policies", application+".json"), 1<<20)
	if err != nil {
		return p, nil, "", err
	}
	if err = workflow.DecodeStrict(raw, &p); err != nil {
		return p, nil, "", err
	}
	if err = p.validate(); err != nil {
		return p, nil, "", err
	}
	if p.Application != application {
		return p, nil, "", errors.New("policy application identity mismatch")
	}
	return p, raw, PolicySHA256(raw), nil
}

func (p PolicyDocument) validate() error {
	if p.SchemaVersion != 1 || p.Application == "" {
		return errors.New("policy identity is invalid")
	}
	if len(p.Windows) == 0 {
		return errors.New("policy requires at least one window title")
	}
	seenWindow := map[string]bool{}
	for _, w := range p.Windows {
		if w == "" || seenWindow[w] {
			return errors.New("policy windows must be non-empty and unique")
		}
		seenWindow[w] = true
	}
	if len(p.PermittedStepKinds) == 0 || len(p.PermittedEffects) == 0 {
		return errors.New("policy requires permitted step kinds and effects")
	}
	for _, kind := range p.PermittedStepKinds {
		if kind == "" {
			return errors.New("policy permitted_step_kinds contains an empty value")
		}
	}
	for _, effect := range p.PermittedEffects {
		if effect != "read" && effect != "change" {
			return fmt.Errorf("policy permits unsupported effect %q", effect)
		}
	}
	for _, field := range []struct {
		name string
		list []PolicySelector
	}{
		{"editable_fields", p.EditableFields},
		{"read_only_targets", p.ReadOnlyTargets},
		{"sensitive_targets", p.SensitiveTargets},
	} {
		for _, s := range field.list {
			if err := s.validate(); err != nil {
				return fmt.Errorf("policy %s: %w", field.name, err)
			}
		}
	}
	return nil
}

func (s PolicySelector) validate() error {
	if s.Name == "" && s.Identifier == "" && s.Visual == nil {
		return errors.New("selector needs a name, identifier, or visual anchor")
	}
	if s.Visual != nil {
		if err := s.Visual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s PolicySelector) equal(other PolicySelector) bool {
	if s.Role != other.Role || s.Name != other.Name || s.Identifier != other.Identifier || s.Ancestor != other.Ancestor {
		return false
	}
	return visualEqual(s.Visual, other.Visual)
}

// visualEqual treats geometry as part of the allowlist identity. The digest
// only names the template; a larger search region or a different click point
// is a different selector.
func visualEqual(a, b *workflow.VisualAnchor) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s PolicySelector) label() string {
	if s.Name != "" {
		return s.Name
	}
	if s.Identifier != "" {
		return s.Identifier
	}
	if s.Visual != nil {
		return "visual:" + s.Visual.SHA256
	}
	return s.Role
}

func (s PolicySelector) toComputer() computer.Selector {
	return computer.Selector{Visual: s.Visual, Role: s.Role, Name: s.Name, Identifier: s.Identifier, AncestorName: s.Ancestor}
}

func policySelectorFromComputer(s computer.Selector) PolicySelector {
	return PolicySelector{Visual: s.Visual, Role: s.Role, Name: s.Name, Identifier: s.Identifier, Ancestor: s.AncestorName}
}

func policySelectorFromWorkflow(s workflow.Selector) PolicySelector {
	primary := s.Primary()
	return PolicySelector{Visual: primary.Visual, Role: primary.Role, Name: primary.Name, Identifier: primary.Identifier, Ancestor: primary.Ancestor}
}

func (p PolicyDocument) containsSelector(list []PolicySelector, want PolicySelector) bool {
	for _, allowed := range list {
		if allowed.equal(want) {
			return true
		}
	}
	return false
}

func (p PolicyDocument) AllowsWindow(title string) bool {
	return slices.Contains(p.Windows, title)
}

func (p PolicyDocument) AllowsReadOnly(s computer.Selector) bool {
	return p.containsSelector(p.ReadOnlyTargets, policySelectorFromComputer(s))
}

func (p PolicyDocument) AllowsEditable(s workflow.Selector) bool {
	return p.containsSelector(p.EditableFields, policySelectorFromWorkflow(s))
}

func (p PolicyDocument) AllowsStepKind(kind string) bool {
	return slices.Contains(p.PermittedStepKinds, kind)
}

func (p PolicyDocument) AllowsEffect(effect string) bool {
	return slices.Contains(p.PermittedEffects, effect)
}

func (p PolicyDocument) SensitiveComputerSelectors() []computer.Selector {
	out := make([]computer.Selector, 0, len(p.SensitiveTargets))
	for _, s := range p.SensitiveTargets {
		out = append(out, s.toComputer())
	}
	return out
}

// AssertNarrowed refuses a candidate policy that widens the reviewed base.
// Profiles and overlays must pass through this before admission.
func (base PolicyDocument) AssertNarrowed(candidate PolicyDocument) error {
	if candidate.SchemaVersion != base.SchemaVersion {
		return errors.New("narrowed policy schema_version mismatch")
	}
	if candidate.Application != base.Application {
		return errors.New("narrowed policy application mismatch")
	}
	if candidate.UnattendedChange && !base.UnattendedChange {
		return errors.New("narrowed policy widens unattended_change")
	}
	for _, w := range candidate.Windows {
		if !base.AllowsWindow(w) {
			return fmt.Errorf("narrowed policy widens windows with %q", w)
		}
	}
	for _, s := range candidate.EditableFields {
		if !base.containsSelector(base.EditableFields, s) {
			return fmt.Errorf("narrowed policy widens editable_fields for %s", s.label())
		}
	}
	for _, s := range candidate.ReadOnlyTargets {
		if !base.containsSelector(base.ReadOnlyTargets, s) {
			return fmt.Errorf("narrowed policy widens read_only_targets for %s", s.label())
		}
	}
	// Sensitive coverage is a protection list: omitting a base entry widens
	// exposure. Candidates may add protections, never remove them.
	for _, s := range base.SensitiveTargets {
		if !candidate.containsSelector(candidate.SensitiveTargets, s) {
			return fmt.Errorf("narrowed policy removes sensitive_targets protection for %s", s.label())
		}
	}
	for _, kind := range candidate.PermittedStepKinds {
		if !base.AllowsStepKind(kind) {
			return fmt.Errorf("narrowed policy widens permitted_step_kinds with %q", kind)
		}
	}
	for _, effect := range candidate.PermittedEffects {
		if !base.AllowsEffect(effect) {
			return fmt.Errorf("narrowed policy widens permitted_effects with %q", effect)
		}
	}
	return nil
}
