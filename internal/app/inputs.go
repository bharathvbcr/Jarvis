package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// parameterName is the capability parameter spelling workflow.Compile accepts.
var parameterName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,95}$`)

// inputsPolicy is the reviewed catalog for an inputs application. It has no
// window or desktop allowlist, so it cannot be the bank policy and the bank
// policy cannot be decoded as one.
type inputsPolicy struct {
	SchemaVersion int                  `json:"schema_version"`
	Application   string               `json:"application"`
	Parameters    []workflow.Parameter `json:"parameters"`
}

type inputsBinding struct {
	Profile     Profile
	Policy      inputsPolicy
	PolicyBytes []byte
}

type inputsDocument struct {
	SchemaVersion int                       `json:"schema_version"`
	Application   string                    `json:"application"`
	Values        map[string]workflow.Value `json:"values"`
}

type inputsPutRequest struct {
	Application string         `json:"application"`
	Key         string         `json:"key"`
	Value       workflow.Value `json:"value"`
}

type inputsKeyRequest struct {
	Application string `json:"application"`
	Key         string `json:"key"`
}

type inputsListRequest struct {
	Application string `json:"application"`
}

// An inputs profile is identified by kind "inputs". It is not north or south,
// it has no window, and its policy file is policies/<application>.json decoded
// as an inputs catalog rather than the bank allowlist.
func (a *App) inputsProfile(application string) (inputsBinding, error) {
	var out inputsBinding
	if err := validInputsApplication(application); err != nil {
		return out, err
	}
	raw, err := readBounded(filepath.Join(a.cfg.Root, "profiles", application+".json"), 1<<20)
	if err != nil {
		return out, err
	}
	var p Profile
	if err = workflow.DecodeStrict(raw, &p); err != nil {
		return out, err
	}
	if p.SchemaVersion != 1 || p.Application != application || p.Tenant != application {
		return out, errors.New("inputs profile identity mismatch")
	}
	if p.Kind != ProfileKindInputs {
		return out, errors.New("inputs loader requires kind inputs")
	}
	if p.Tenant == "north" || p.Tenant == "south" {
		return out, errors.New("inputs profile is not a bank tenant")
	}
	if p.WindowTitle != "" || len(p.ReadOnlyTargets) != 0 || p.Overlay != "" || len(p.Overlays) != 0 {
		return out, errors.New("inputs profile has no window, broker target, or discovery overlay")
	}
	if p.Policy == "" {
		p.Policy = p.Application
	}
	if p.Policy != p.Application || p.Policy == BankApplication {
		return out, errors.New("inputs profile cannot reuse the bank policy")
	}
	policy, policyBytes, err := a.loadInputsPolicy(p.Policy)
	if err != nil {
		return out, err
	}
	if err = validateParameterList(p.Parameters, true); err != nil {
		return out, err
	}
	if err = validateParameterList(policy.Parameters, true); err != nil {
		return out, err
	}
	if !sameParameters(p.Parameters, policy.Parameters) {
		return out, errors.New("inputs profile widens the reviewed parameter catalog")
	}
	out = inputsBinding{Profile: p, Policy: policy, PolicyBytes: policyBytes}
	return out, nil
}

func validInputsApplication(id string) error {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, `\`) || strings.Contains(id, "..") || !parameterName.MatchString(id) {
		return errors.New("invalid inputs application")
	}
	if id == BankApplication {
		return errors.New("inputs application cannot be jarvis-bank")
	}
	return nil
}

func (a *App) loadInputsPolicy(application string) (inputsPolicy, []byte, error) {
	var p inputsPolicy
	if err := validInputsApplication(application); err != nil {
		return p, nil, err
	}
	raw, err := readBounded(filepath.Join(a.cfg.Root, "policies", application+".json"), 1<<20)
	if err != nil {
		return p, nil, err
	}
	// The bank document has windows and targets. Strict decoding refuses those
	// fields, so this loader cannot accept policies/jarvis-bank.json.
	if err = workflow.DecodeStrict(raw, &p); err != nil {
		return p, nil, fmt.Errorf("inputs policy refused: %w", err)
	}
	if p.SchemaVersion != 1 || p.Application != application {
		return p, nil, errors.New("inputs policy identity mismatch")
	}
	return p, raw, nil
}

func validateParameterList(params []workflow.Parameter, inputsCatalog bool) error {
	if inputsCatalog && len(params) == 0 {
		return errors.New("inputs profile declares no parameters")
	}
	seen := map[string]bool{}
	for _, p := range params {
		if !parameterName.MatchString(p.Name) || seen[p.Name] {
			return fmt.Errorf("invalid parameter %q", p.Name)
		}
		seen[p.Name] = true
		switch p.Type {
		case "string", "integer", "boolean", "money":
		default:
			return fmt.Errorf("parameter %s has unsupported type %q", p.Name, p.Type)
		}
		if !inputsCatalog {
			continue
		}
		if p.Type != "string" || !p.Sensitive {
			return fmt.Errorf("inputs parameter %s must be a sensitive string", p.Name)
		}
		if p.Name == "member_id" || p.Name == "subaccount_name" {
			return fmt.Errorf("inputs profile reuses bank parameter %s", p.Name)
		}
	}
	return nil
}

func sameParameters(a, b []workflow.Parameter) bool {
	if len(a) != len(b) {
		return false
	}
	got := make(map[string]workflow.Parameter, len(a))
	for _, p := range a {
		got[p.Name] = p
	}
	for _, p := range b {
		if got[p.Name] != p {
			return false
		}
	}
	return true
}

// sensitiveInputs admits only names the profile declares. Sensitivity is the
// profile's flag, not a fixed member_id/subaccount_name pair.
func sensitiveInputs(profile Profile, inputs map[string]workflow.Value) ([]workflow.Parameter, error) {
	declared := make(map[string]workflow.Parameter, len(profile.Parameters))
	for _, p := range profile.Parameters {
		if _, dup := declared[p.Name]; dup || p.Name == "" {
			return nil, errors.New("profile declares an invalid parameter")
		}
		declared[p.Name] = p
	}
	for name, value := range inputs {
		param, ok := declared[name]
		if !ok {
			return nil, errors.New("unknown parameter")
		}
		if value.Type != param.Type {
			return nil, fmt.Errorf("parameter %s has type %s", name, value.Type)
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
	}
	out := make([]workflow.Parameter, 0, len(inputs))
	for _, param := range profile.Parameters {
		if _, ok := inputs[param.Name]; !ok {
			continue
		}
		out = append(out, workflow.Parameter{Name: param.Name, Type: param.Type, Sensitive: param.Sensitive})
	}
	return out, nil
}

func (a *App) inputsPut(ctx context.Context, application, key string, value workflow.Value) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	binding, err := a.inputsProfile(application)
	if err != nil {
		return err
	}
	param, ok := parameterByName(binding.Profile.Parameters, key)
	if !ok || value.Type != param.Type || !param.Sensitive {
		return errors.New("unknown inputs parameter")
	}
	if strings.TrimSpace(value.Text) == "" {
		return errors.New("inputs value is empty")
	}
	if err = value.Validate(); err != nil {
		return err
	}
	a.inputsMu.Lock()
	defer a.inputsMu.Unlock()
	if err = ctx.Err(); err != nil {
		return err
	}
	doc, err := a.readInputsStore(application)
	if errors.Is(err, os.ErrNotExist) {
		doc = inputsDocument{SchemaVersion: 1, Application: application, Values: map[string]workflow.Value{}}
		err = nil
	}
	if err != nil {
		return err
	}
	if err = doc.matches(application, binding.Profile.Parameters); err != nil {
		return err
	}
	line := inputsLogLine("put", application, key, a.actor())
	if doc.Values == nil {
		doc.Values = map[string]workflow.Value{}
	}
	doc.Values[key] = value
	if err = a.writeInputsStore(application, doc); err != nil {
		return err
	}
	return a.appendInputsLog(line)
}

func (a *App) inputsGet(ctx context.Context, application, key string) (workflow.Value, error) {
	if err := ctx.Err(); err != nil {
		return workflow.Value{}, err
	}
	binding, err := a.inputsProfile(application)
	if err != nil {
		return workflow.Value{}, err
	}
	if _, ok := parameterByName(binding.Profile.Parameters, key); !ok {
		return workflow.Value{}, errors.New("unknown inputs parameter")
	}
	// A missing bank executable is not an empty catalog. Fail the read before
	// looking at the store so the caller cannot mistake "the binary is absent"
	// for "this key has no value".
	if err = a.requireBankExecutable(); err != nil {
		return workflow.Value{}, err
	}
	a.inputsMu.Lock()
	defer a.inputsMu.Unlock()
	if err = ctx.Err(); err != nil {
		return workflow.Value{}, err
	}
	doc, err := a.readInputsStore(application)
	if errors.Is(err, os.ErrNotExist) {
		return workflow.Value{}, errors.New("inputs value is not stored")
	}
	if err != nil {
		return workflow.Value{}, err
	}
	if err = doc.matches(application, binding.Profile.Parameters); err != nil {
		return workflow.Value{}, err
	}
	value, ok := doc.Values[key]
	if !ok || value.Text == "" {
		return workflow.Value{}, errors.New("inputs value is not stored")
	}
	if err = a.appendInputsLog(inputsLogLine("get", application, key, a.actor())); err != nil {
		return workflow.Value{}, err
	}
	return value, nil
}

func (a *App) inputsList(ctx context.Context, application string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binding, err := a.inputsProfile(application)
	if err != nil {
		return nil, err
	}
	if err = a.requireBankExecutable(); err != nil {
		return nil, err
	}
	a.inputsMu.Lock()
	defer a.inputsMu.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	doc, err := a.readInputsStore(application)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err = doc.matches(application, binding.Profile.Parameters); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(doc.Values))
	for name := range doc.Values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	if err = a.appendInputsLog(inputsLogLine("list-keys", application, "", a.actor())); err != nil {
		return nil, err
	}
	return keys, nil
}

func (a *App) requireBankExecutable() error {
	info, err := os.Lstat(a.cfg.Bank)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("inputs read failed: bank executable is missing")
	}
	return nil
}

func parameterByName(params []workflow.Parameter, name string) (workflow.Parameter, bool) {
	for _, p := range params {
		if p.Name == name {
			return p, true
		}
	}
	return workflow.Parameter{}, false
}

func (d inputsDocument) matches(application string, catalog []workflow.Parameter) error {
	if d.SchemaVersion != 1 || d.Application != application {
		return errors.New("inputs store identity mismatch")
	}
	for name, value := range d.Values {
		param, ok := parameterByName(catalog, name)
		if !ok || value.Type != param.Type || !param.Sensitive {
			return errors.New("inputs store contains a parameter outside the catalog")
		}
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) inputsStorePath(application string) string {
	return filepath.Join(a.cfg.Root, ".local", "inputs", application+".json")
}

func (a *App) readInputsStore(application string) (inputsDocument, error) {
	var doc inputsDocument
	raw, err := readBounded(a.inputsStorePath(application), 1<<20)
	if err != nil {
		return doc, err
	}
	if err = workflow.DecodeStrict(raw, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func (a *App) writeInputsStore(application string, doc inputsDocument) error {
	doc.SchemaVersion = 1
	doc.Application = application
	return writeJSON(a.inputsStorePath(application), doc)
}

// inputsLogLine is the serve log entry for one operation. A stored value never
// enters the line: removing it afterwards by search-and-replace fails when
// stored values overlap each other or the fixed text, so the line is built from
// the catalog-validated application and key plus a fixed marker for the value.
// actor names who an audit line is for: "agent" for a yolo-mode operator,
// "person" otherwise, so an agent's inputs.put is attributable after the fact.
func (a *App) actor() string {
	if a.cfg.Operator != "" {
		return a.cfg.Operator
	}
	return "person"
}

func inputsLogLine(op, application, key, actor string) string {
	line := fmt.Sprintf("op=%s actor=%s application=%s key=%s", op, actor, application, key)
	if op != "list-keys" {
		line += " value=[redacted]"
	}
	return line
}

func (a *App) appendInputsLog(line string) error {
	dir := filepath.Join(a.cfg.Root, ".local", "inputs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "serve.log")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("inputs serve log is not a regular file")
		}
		if info.Size() > 1<<20 {
			if err = os.Remove(path); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.WriteString(line + "\n"); err != nil {
		return err
	}
	return f.Sync()
}

func (a *App) configureInputs(r *serve.Router) error {
	ops := map[string]serve.Handler{
		"jarvis.inputs.put": handler(func(ctx context.Context, p inputsPutRequest) (struct {
			Stored bool `json:"stored"`
		}, error) {
			err := a.inputsPut(ctx, p.Application, p.Key, p.Value)
			return struct {
				Stored bool `json:"stored"`
			}{err == nil}, err
		}),
		"jarvis.inputs.list-keys": handler(func(ctx context.Context, p inputsListRequest) (struct {
			Keys []string `json:"keys"`
		}, error) {
			keys, err := a.inputsList(ctx, p.Application)
			return struct {
				Keys []string `json:"keys"`
			}{keys}, err
		}),
		"jarvis.inputs.get": handler(func(ctx context.Context, p inputsKeyRequest) (struct {
			Key   string         `json:"key"`
			Value workflow.Value `json:"value"`
		}, error) {
			value, err := a.inputsGet(ctx, p.Application, p.Key)
			return struct {
				Key   string         `json:"key"`
				Value workflow.Value `json:"value"`
			}{p.Key, value}, err
		}),
	}
	for name, h := range ops {
		if err := r.Register(name, h); err != nil {
			return err
		}
	}
	return nil
}
