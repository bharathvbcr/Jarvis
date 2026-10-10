//go:build !windows

package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stub writes an executable that prints stdout and exits with code.
func stub(t *testing.T, stdout string, code int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dcverify")
	body := "#!/bin/sh\ncat <<'EOF'\n" + stdout + "\nEOF\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func request(binary string) Request {
	sha := strings.Repeat("a", 64)
	return Request{Binary: binary, ContractPath: "c.json", BundlePath: "b.json", ArtifactsRoot: ".", ContractSHA256: sha, CapabilitySHA256: strings.Repeat("b", 64), RunID: "run-1", SessionID: "s-1", Epoch: 1}
}

const passed = `{"ok":true,"evidence_schema_version":1,"verdict":"passed","contract_sha256":"` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `","bundle_sha256":"x","run_id":"run-1","criteria":[],"issues":[]}`

func TestVerifyAcceptsAMatchingPassedReport(t *testing.T) {
	report, err := Verify(context.Background(), request(stub(t, passed, 0)))
	if err != nil || report.Verdict != "passed" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestVerifyFailsClosed(t *testing.T) {
	for name, tc := range map[string]struct {
		binary string
		mutate func(*Request)
	}{
		"incomplete expectations":   {stub(t, passed, 0), func(r *Request) { r.RunID = "" }},
		"zero epoch":                {stub(t, passed, 0), func(r *Request) { r.Epoch = 0 }},
		"short contract digest":     {stub(t, passed, 0), func(r *Request) { r.ContractSHA256 = "abc" }},
		"no output":                 {stub(t, "", 0), nil},
		"wrong run":                 {stub(t, strings.Replace(passed, `"run-1"`, `"forged"`, 1), 0), nil},
		"unknown field":             {stub(t, strings.Replace(passed, `"ok":true`, `"ok":true,"extra":1`, 1), 0), nil},
		"trailing data":             {stub(t, passed+passed, 0), nil},
		"bad verdict":               {stub(t, strings.Replace(passed, `"passed"`, `"fine"`, 1), 0), nil},
		"passed but process failed": {stub(t, passed, 1), nil},
		"exit above 2":              {stub(t, strings.Replace(passed, `"passed"`, `"failed"`, 1), 3), nil},
		"missing binary":            {"/nonexistent/dcverify", nil},
	} {
		t.Run(name, func(t *testing.T) {
			r := request(tc.binary)
			if tc.mutate != nil {
				tc.mutate(&r)
			}
			if report, err := Verify(context.Background(), r); err == nil {
				t.Fatalf("accepted: %+v", report)
			}
		})
	}
}

func TestVerifyHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Verify(ctx, request(stub(t, passed, 0))); err == nil {
		t.Fatal("cancelled context accepted")
	}
}

func TestVerifyBoundsVerifierOutput(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", 1<<20+10) + `"}`
	if _, err := Verify(context.Background(), request(stub(t, big, 0))); err == nil {
		t.Fatal("oversized verifier output accepted")
	}
}
