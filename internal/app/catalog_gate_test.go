package app

import (
	"context"
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
