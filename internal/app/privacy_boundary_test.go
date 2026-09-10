package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestMain(m *testing.M) {
	if os.Getenv("JARVIS_OWNED_BANK_FIXTURE") == "1" && len(os.Args) > 1 && os.Args[1] == "bank-fixture" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if os.Getenv("JARVIS_DIAGNOSTIC_BROKER") != "1" || len(os.Args) < 2 || os.Args[1] != "serve" {
		os.Exit(m.Run())
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 100, 80))); err != nil {
		os.Exit(2)
	}
	value := "synthetic-private-member"
	balance := "$1.00"
	bounds := computer.Bounds{X: 10, Y: 10, Width: 30, Height: 10}
	window := computer.Window{PID: 1, ID: 2, Title: "Diagnostic bank", ProcessIdentity: "fixture", Bounds: computer.Bounds{Width: 100, Height: 80}, Foreground: true, Scale: 1}
	nodes := []computer.Node{{ID: "member", Name: "Member ID", Role: "text_field", Value: &value, Bounds: &bounds}, {ID: "balance", Name: "Balance", Role: "text_field", Value: &balance, Bounds: &bounds}}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	attachAttempts := 0
	for scanner.Scan() {
		var r struct {
			ID, Op string
			RunID  string `json:"run_id"`
			Epoch  uint64 `json:"epoch"`
		}
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			os.Exit(2)
		}
		if path := os.Getenv("JARVIS_DIAGNOSTIC_LOG"); path != "" {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				os.Exit(4)
			}
			_, writeErr := f.WriteString(r.Op + "\n")
			if err := f.Close(); err != nil || writeErr != nil {
				os.Exit(4)
			}
		}
		var result json.RawMessage
		var encodeErr error
		switch r.Op {
		case "probe":
			if os.Getenv("JARVIS_DIAGNOSTIC_PROBE_DELAY") == "1" {
				time.Sleep(2 * time.Second)
			}
			result = []byte(`{"qualification":"fixture"}`)
		case "attach":
			attachAttempts++
			if os.Getenv("JARVIS_DIAGNOSTIC_TRANSIENT_ATTACH") == "1" && attachAttempts == 1 {
				if encoder.Encode(struct {
					ID    string               `json:"id"`
					OK    bool                 `json:"ok"`
					Error computer.BrokerError `json:"error"`
				}{r.ID, false, computer.BrokerError{Code: "window_missing", Delivery: "not_sent", Message: "fixture window is starting"}}) != nil {
					os.Exit(3)
				}
				continue
			}
			result, encodeErr = json.Marshal(computer.Session{ID: "s", RunID: r.RunID, Epoch: 1, Window: window})
		case "observe":
			if os.Getenv("JARVIS_DIAGNOSTIC_BLOCK") == "1" {
				time.Sleep(10 * time.Second)
			}
			result, encodeErr = json.Marshal(computer.Observation{ID: "o", Epoch: r.Epoch, Window: window, Nodes: nodes, Complete: true, Screenshot: computer.Screenshot{MIMEType: "image/png", Base64: base64.StdEncoding.EncodeToString(b.Bytes()), Width: 100, Height: 80}})
		case "resolve":
			result, encodeErr = json.Marshal(struct {
				TargetID string        `json:"target_id"`
				Node     computer.Node `json:"node"`
			}{"balance", nodes[1]})
		default:
			result = []byte(`{"epoch":2}`)
		}
		if encodeErr != nil {
			os.Exit(3)
		}
		if encoder.Encode(struct {
			ID     string          `json:"id"`
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}{r.ID, true, result}) != nil {
			os.Exit(3)
		}
	}
	os.Exit(0)
}
func TestCapabilityCannotDisableTrustedFieldPrivacy(t *testing.T) {
	a, req := diagnosticApp(t, "Diagnostic bank")
	spec := workflow.Capability{SchemaVersion: 1, ID: "probe", Revision: "1", Application: "jarvis-bank", Parameters: []workflow.Parameter{{Name: "member_id", Type: "string", Sensitive: false}}, Targets: map[string]workflow.Selector{"balance": {Name: "Balance", Role: "text_field"}}, Limits: workflow.DefaultLimits(), Steps: []workflow.Step{{ID: "balance", Kind: "extract", Target: "balance", Effect: "read", Output: "balance", OutputType: "money", Currency: "USD"}}}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.cfg.Root, "capability.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	req.Inputs = map[string]workflow.Value{"member_id": {Type: "string", Text: "synthetic-private-member"}}
	id, err := a.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	v, err := a.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Observation == nil {
		t.Fatal("diagnostic failed before capture:", v.Error)
	}
	raw, err = json.Marshal(v.Observation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic-private-member") {
		t.Fatal("capability Sensitive=false disabled trusted Member ID privacy in the real App.execute path")
	}
}

func diagnosticApp(t *testing.T, title string) (*App, StartRequest) {
	t.Helper()
	t.Setenv("JARVIS_DIAGNOSTIC_BROKER", "1")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "profiles"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "policies"), 0700); err != nil {
		t.Fatal(err)
	}
	policyRaw, err := os.ReadFile("../../policies/jarvis-bank.json")
	if err != nil {
		t.Fatal(err)
	}
	var policy PolicyDocument
	if err = json.Unmarshal(policyRaw, &policy); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range policy.Windows {
		if w == title {
			found = true
			break
		}
	}
	if !found {
		policy.Windows = append(policy.Windows, title)
	}
	policyRaw, err = json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "policies", "jarvis-bank.json"), policyRaw, 0600); err != nil {
		t.Fatal(err)
	}
	targets := make([]computer.Selector, 0, len(policy.ReadOnlyTargets))
	for _, s := range policy.ReadOnlyTargets {
		targets = append(targets, s.toComputer())
	}
	profile := Profile{SchemaVersion: 1, Tenant: "north", Application: "jarvis-bank", Policy: "jarvis-bank", Platforms: []string{runtime.GOOS}, WindowTitle: title, ReadOnlyTargets: targets}
	raw, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "profiles", "north.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	c := workflow.Capability{SchemaVersion: 1, ID: "probe", Revision: "1", Application: "jarvis-bank", Targets: map[string]workflow.Selector{"balance": {Name: "Balance", Role: "text_field"}}, Limits: workflow.DefaultLimits(), Steps: []workflow.Step{{ID: "balance", Kind: "extract", Target: "balance", Effect: "read", Output: "balance", OutputType: "money", Currency: "USD"}}}
	raw, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "capability.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "contract.json"), []byte(`{"schema_version":1,"id":"test","criteria":[{"id":"balance","fact":"balance_minor","required":true,"predicate":{"op":"exists"}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root, Broker: os.Args[0], Verifier: filepath.Join(root, "absent-verifier")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a, StartRequest{CapabilityPath: "capability.json", ContractPath: "contract.json", Tenant: "north", PID: 1}
}
func TestMismatchedWindowIsDetachedBeforeRunRetires(t *testing.T) {
	log := filepath.Join(t.TempDir(), "broker-ops")
	t.Setenv("JARVIS_DIAGNOSTIC_LOG", log)
	a, req := diagnosticApp(t, "Different expected title")
	id, err := a.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	view, err := a.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.Error, "profile") {
		t.Fatalf("window mismatch did not fail: %s", view.Error)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "detach\n") {
		t.Fatalf("attached session was not detached before retirement: %s", raw)
	}
}
func TestCloseCancelsBlockedRunAndRetiresAdmission(t *testing.T) {
	log := filepath.Join(t.TempDir(), "broker-ops")
	t.Setenv("JARVIS_DIAGNOSTIC_LOG", log)
	t.Setenv("JARVIS_DIAGNOSTIC_BLOCK", "1")
	a, req := diagnosticApp(t, "Diagnostic bank")
	id, err := a.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		raw, err := os.ReadFile(log)
		if err == nil && strings.Contains(string(raw), "observe\n") {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("fixture never began capture")
		case <-time.After(time.Millisecond):
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- a.Close() }()
	select {
	case <-ctx.Done():
		t.Fatal("Close blocked behind active capture")
	case <-closed:
	}
	view, err := a.Wait(ctx, id)
	if err != nil || !view.Finished {
		t.Fatalf("close did not retire active run: %+v %v", view, err)
	}
	if _, err := a.Start(req); err == nil {
		t.Fatal("closed app admitted another run")
	}
}
