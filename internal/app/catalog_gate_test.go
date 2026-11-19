package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireApprovedForUnattendedRefusesDraft(t *testing.T) {
	root := t.TempDir()
	capPath := filepath.Join(root, "cap.json")
	raw := []byte(`{"schema_version":1,"id":"bank","revision":"1","application":"bank","targets":{"balance":{"name":"Balance"}},"steps":[{"id":"balance","kind":"extract","target":"balance","effect":"read","output":"balance","output_type":"money","currency":"USD"}],"limits":{"max_actions":40,"active_seconds":300,"observation_attempts":3,"intervention_seconds":600}}`)
	if err := os.WriteFile(capPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.RequireApprovedForUnattended(capPath); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("missing catalog entry: %v", err)
	}
	p, err := a.Compile(capPath)
	if err != nil {
		t.Fatal(err)
	}
	e, err := a.Catalog().Put(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RequireApprovedForUnattended(capPath); err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("draft: %v", err)
	}
	if err := a.Catalog().Approve(e.ID, e.Revision, e.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := a.RequireApprovedForUnattended(capPath); err != nil {
		t.Fatal(err)
	}
	if err := a.Catalog().Revoke(e.ID, e.Revision, e.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := a.RequireApprovedForUnattended(capPath); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("revoked: %v", err)
	}
}

// An agent's yolo promotion sets the catalog status to approved like a person's,
// and only the promotion history says who did it. Unattended replay reads it: a
// revision an agent promoted last is refused until a person promotes it.
func TestRequireApprovedForUnattendedRefusesARevisionAnAgentPromoted(t *testing.T) {
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	capPath := filepath.Join(root, "balance.json")
	if err := os.WriteFile(capPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	source, _ := json.Marshal(map[string]any{"source": string(raw), "register": true})

	agent, err := New(context.Background(), Config{Root: root, Operator: OperatorAgent})
	if err != nil {
		t.Fatal(err)
	}
	if r := serveCall(t, agent, "jarvis.capability.compile", string(source)); !r.OK {
		t.Fatalf("register: %+v", r.Error)
	}
	if r := serveCall(t, agent, "jarvis.catalog.promote", `{"id":"bank.balance","revision":"1"}`); !r.OK {
		t.Fatalf("agent promote: %+v", r.Error)
	}
	agent.Close()

	person, err := New(context.Background(), Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer person.Close()
	if err := person.RequireApprovedForUnattended(capPath); err == nil || !strings.Contains(err.Error(), "promoted by agent") {
		t.Fatalf("an agent-promoted revision was admitted unattended: %v", err)
	}
	if r := serveCall(t, person, "jarvis.catalog.promote", `{"id":"bank.balance","revision":"1"}`); !r.OK {
		t.Fatalf("person promote: %+v", r.Error)
	}
	if err := person.RequireApprovedForUnattended(capPath); err != nil {
		t.Fatalf("a revision a person promoted again is still refused: %v", err)
	}
}
