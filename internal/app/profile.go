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
// reviewed bank policy compiled into this product host.
type Profile struct {
	SchemaVersion   int                 `json:"schema_version"`
	Tenant          string              `json:"tenant"`
	Application     string              `json:"application"`
	Platforms       []string            `json:"platforms"`
	WindowTitle     string              `json:"window_title"`
	ReadOnlyTargets []computer.Selector `json:"read_only_targets"`
}

func (a *App) profile(tenant string) (Profile, []byte, error) {
	var p Profile
	if tenant != "north" && tenant != "south" {
		return p, nil, errors.New("unknown tenant binding")
	}
	raw, err := readBounded(filepath.Join(a.cfg.Root, "profiles", tenant+".json"), 1<<20)
	if err != nil {
		return p, nil, err
	}
	if err = workflow.DecodeStrict(raw, &p); err != nil {
		return p, nil, err
	}
	if p.SchemaVersion != 1 || p.Tenant != tenant || p.Application != "jarvis-bank" || p.WindowTitle == "" {
		return p, nil, errors.New("profile identity mismatch")
	}
	supported := false
	for _, platform := range p.Platforms {
		if platform == runtime.GOOS {
			supported = true
		}
	}
	if !supported {
		return p, nil, errors.New("profile does not support this platform")
	}
	for _, target := range p.ReadOnlyTargets {
		found := false
		for _, allowed := range bankReadTargets() {
			if target == allowed {
				found = true
			}
		}
		if !found {
			return p, nil, fmt.Errorf("binding attempts to widen input policy for %s", target.Name)
		}
	}
	return p, raw, nil
}
func validateBankProgram(p *workflow.Program, profile Profile) error {
	for _, step := range p.Capability().Steps {
		selector := p.Capability().Targets[step.Target]
		if step.Kind == "extract" || step.Kind == "assert" || step.Kind == "branch" || step.Kind == "wait" {
			continue
		}
		if step.Effect == "change" {
			continue
		}
		if step.Kind == "set_value" || step.Kind == "type_text" {
			if selector.Role != "text_field" || (selector.Name != "Member ID" && selector.Name != "Subaccount name") {
				return errors.New("binding permits value entry only in reviewed editable fields")
			}
		}
		found := false
		for _, allowed := range profile.ReadOnlyTargets {
			if selector.Role == allowed.Role && selector.Name == allowed.Name && selector.Identifier == allowed.Identifier && selector.Ancestor == allowed.AncestorName {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("read-effect step %s lacks a trusted profile binding", step.ID)
		}
	}
	return nil
}

// The trusted application boundary owns privacy. Capability metadata can ask
// for additional protection; it cannot make bank entry fields public.
func bankPrivacy(inputs map[string]workflow.Value) computer.PrivacyPolicy {
	p := computer.PrivacyPolicy{SensitiveTargets: []computer.Selector{{Role: "text_field", Name: "Member ID"}, {Role: "text_field", Name: "Subaccount name"}}}
	for _, v := range inputs {
		if v.Type == "string" && v.Text != "" {
			p.Watched = append(p.Watched, v.Text)
		}
	}
	return p
}
