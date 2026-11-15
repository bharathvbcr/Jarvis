package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestInputsProfileIsTheVaniCatalog(t *testing.T) {
	a, err := New(context.Background(), Config{Root: "../..", Bank: filepath.Join(t.TempDir(), "missing-bank")})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	binding, err := a.inputsProfile(InputsApplication)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Profile.Kind != ProfileKindInputs || binding.Profile.WindowTitle != "" {
		t.Fatalf("profile = %+v", binding.Profile)
	}
	want := []string{"given_name", "family_name", "email", "phone", "city", "country", "linkedin", "github", "website"}
	if len(binding.Profile.Parameters) != len(want) {
		t.Fatalf("parameters = %+v", binding.Profile.Parameters)
	}
	for i, name := range want {
		p := binding.Profile.Parameters[i]
		if p.Name != name || p.Type != "string" || !p.Sensitive {
			t.Fatalf("parameter %d = %+v", i, p)
		}
		if p.Name == "member_id" || p.Name == "subaccount_name" {
			t.Fatalf("inputs catalog reuses %s", p.Name)
		}
	}
	bank, err := os.ReadFile("../../policies/jarvis-bank.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(binding.PolicyBytes) == string(bank) {
		t.Fatal("inputs policy reuses jarvis-bank.json")
	}
	var asBank PolicyDocument
	if err = workflow.DecodeStrict(binding.PolicyBytes, &asBank); err == nil {
		t.Fatal("inputs policy decoded as the bank allowlist")
	}
	var asInputs inputsPolicy
	if err = workflow.DecodeStrict(bank, &asInputs); err == nil {
		t.Fatal("bank policy decoded as an inputs catalog")
	}
}

func TestInputsProfileRejectsTheBankPolicyAndAWiderCatalog(t *testing.T) {
	root := t.TempDir()
	installVaniInputs(t, root)
	bank, err := os.ReadFile("../../policies/jarvis-bank.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "policies", "vani.json"), bank, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err = a.inputsProfile(InputsApplication); err == nil {
		t.Fatal("inputs profile accepted policies/jarvis-bank.json")
	}

	installVaniInputs(t, root)
	raw, err := os.ReadFile(filepath.Join(root, "profiles", "vani.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile Profile
	if err = json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	profile.Parameters = append(profile.Parameters, workflow.Parameter{Name: "middle_name", Type: "string", Sensitive: true})
	raw, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "vani.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = a.inputsProfile(InputsApplication); err == nil || !strings.Contains(err.Error(), "widens") {
		t.Fatalf("inputs profile widened the catalog: %v", err)
	}
	profile.Parameters[len(profile.Parameters)-1] = workflow.Parameter{Name: "member_id", Type: "string", Sensitive: true}
	raw, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "vani.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = a.inputsProfile(InputsApplication); err == nil || !strings.Contains(err.Error(), "member_id") {
		t.Fatalf("inputs profile reused member_id: %v", err)
	}

	installVaniInputs(t, root)
	raw, err = os.ReadFile(filepath.Join(root, "profiles", "vani.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	profile.WindowTitle = "Vani"
	raw, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "profiles", "vani.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = a.inputsProfile(InputsApplication); err == nil || !strings.Contains(err.Error(), "window") {
		t.Fatalf("inputs profile accepted a window title: %v", err)
	}
}

func TestInputsReadWithoutBankExecutableFails(t *testing.T) {
	root := t.TempDir()
	installVaniInputs(t, root)
	const secret = "contact-secret-example-9f3a"
	a, err := New(context.Background(), Config{Root: root, Bank: filepath.Join(root, "missing-bank")})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.inputsPut(context.Background(), InputsApplication, "email", workflow.Value{Type: "string", Text: secret}); err != nil {
		t.Fatal(err)
	}
	value, err := a.inputsGet(context.Background(), InputsApplication, "email")
	if err == nil {
		t.Fatalf("inputs read with no bank executable succeeded: %+v", value)
	}
	if value != (workflow.Value{}) || strings.Contains(err.Error(), secret) {
		t.Fatalf("failed read returned %q via %v", value.Text, err)
	}
	keys, err := a.inputsList(context.Background(), InputsApplication)
	if err == nil {
		t.Fatalf("inputs list with no bank executable succeeded: %v", keys)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("list error contains the stored value")
	}
}

func TestInputsServeRoundTripRedactsTheLog(t *testing.T) {
	root := t.TempDir()
	installVaniInputs(t, root)
	bank := filepath.Join(root, "bank-placeholder")
	if err := os.WriteFile(bank, []byte("not executed"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root, Bank: bank})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	const secret = "contact-secret-example-9f3a"
	const phone = "contact-phone-example-4412"
	put := inputsCall(t, a, "jarvis.inputs.put", map[string]any{
		"application": InputsApplication,
		"key":         "email",
		"value":       map[string]string{"type": "string", "text": secret},
	})
	if !put.OK || strings.Contains(string(put.Result), secret) || string(put.Result) != `{"stored":true}` {
		t.Fatalf("put = %+v %s", put, put.Result)
	}
	phonePut := inputsCall(t, a, "jarvis.inputs.put", map[string]any{
		"application": InputsApplication,
		"key":         "phone",
		"value":       map[string]string{"type": "string", "text": phone},
	})
	if !phonePut.OK {
		t.Fatalf("phone put = %+v", phonePut)
	}
	listed := inputsCall(t, a, "jarvis.inputs.list-keys", map[string]string{"application": InputsApplication})
	if !listed.OK || strings.Contains(string(listed.Result), secret) || strings.Contains(string(listed.Result), phone) {
		t.Fatalf("list = %s", listed.Result)
	}
	var keys struct {
		Keys []string `json:"keys"`
	}
	if err = json.Unmarshal(listed.Result, &keys); err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys.Keys, ",") != "email,phone" {
		t.Fatalf("keys = %v", keys.Keys)
	}
	got := inputsCall(t, a, "jarvis.inputs.get", map[string]string{"application": InputsApplication, "key": "email"})
	if !got.OK || !strings.Contains(string(got.Result), secret) {
		t.Fatalf("get = %+v %s", got, got.Result)
	}
	const replaced = "contact-secret-example-replaced"
	again := inputsCall(t, a, "jarvis.inputs.put", map[string]any{
		"application": InputsApplication,
		"key":         "email",
		"value":       map[string]string{"type": "string", "text": replaced},
	})
	if !again.OK || strings.Contains(string(again.Result), replaced) {
		t.Fatalf("replace = %+v %s", again, again.Result)
	}
	got = inputsCall(t, a, "jarvis.inputs.get", map[string]string{"application": InputsApplication, "key": "email"})
	if !got.OK || !strings.Contains(string(got.Result), replaced) || strings.Contains(string(got.Result), secret) {
		t.Fatalf("replaced get = %s", got.Result)
	}
	logRaw, err := os.ReadFile(filepath.Join(root, ".local", "inputs", "serve.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logRaw), secret) || strings.Contains(string(logRaw), phone) || strings.Contains(string(logRaw), replaced) || !strings.Contains(string(logRaw), "[redacted]") {
		t.Fatalf("serve log did not redact stored values: %s", logRaw)
	}

	rejected := inputsCall(t, a, "jarvis.inputs.put", map[string]any{
		"application": InputsApplication,
		"key":         "member_id",
		"value":       map[string]string{"type": "string", "text": secret},
		"extra":       "no",
	})
	if rejected.OK || strings.Contains(rejected.Error.Message, secret) {
		t.Fatalf("malformed put = %+v", rejected)
	}
	oversized := strings.Repeat("x", 65537)
	if err = a.inputsPut(context.Background(), InputsApplication, "city", workflow.Value{Type: "string", Text: oversized}); err == nil || strings.Contains(err.Error(), oversized[:32]) {
		t.Fatalf("oversized put = %v", err)
	}
	if err = a.inputsPut(context.Background(), InputsApplication, "city", workflow.Value{Type: "string"}); err == nil {
		t.Fatal("empty value stored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = a.inputsGet(ctx, InputsApplication, "email"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled get = %v", err)
	}
}

func installVaniInputs(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{"profiles", "policies"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"profiles/vani.json", "policies/vani.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func inputsCall(t *testing.T, a *App, op string, params any) serve.Response {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer input.Close()
	defer send.Close()
	defer output.Close()
	defer receive.Close()
	server := serve.New(receive, serve.Options{HardRules: true, Modules: []serve.Module{a}})
	go func() { _ = server.Serve(ctx, input) }()
	replies := make(chan serve.Response, 2)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var response serve.Response
			if decoder.Decode(&response) != nil {
				return
			}
			replies <- response
		}
	}()
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		enc := json.NewEncoder(send)
		for _, request := range []serve.Request{
			{ID: "hello", Op: "hello", Params: json.RawMessage(`{"protocol":1,"host":"inputs-test"}`)},
			{ID: "call", Op: op, Params: encoded},
		} {
			if enc.Encode(request) != nil {
				return
			}
		}
	}()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case response := <-replies:
			if response.ID == "hello" {
				if !response.OK {
					t.Fatalf("hello failed: %+v", response)
				}
				continue
			}
			if response.ID != "call" {
				t.Fatalf("unexpected response %+v", response)
			}
			return response
		case <-timer.C:
			t.Fatal(op + " did not answer within 20s")
		}
	}
}
