//go:build darwin || linux

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/credentials"
	"github.com/bharathvbcr/Manvi/manvi/serve"
)

// keySetChildEnv marks the re-executed test binary that runs with no locked
// memory. The limit is process-wide and a failed seal can leave memguard's
// state unusable, so it never runs in the shared test process.
const keySetChildEnv = "JARVIS_KEY_SET_UNSEALABLE_CHILD"

// A key the workbench hands over that cannot be sealed must be refused at
// jarvis.key.set. Reporting configured:true sends the user on to a discovery
// or assist run that then says the credential is unavailable.
func TestKeySetRefusesKeyThatCannotBeSealed(t *testing.T) {
	if os.Getenv(keySetChildEnv) == "1" {
		keySetUnsealableChild(t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeySetRefusesKeyThatCannotBeSealed$", "-test.v", "-test.count=1")
	// The child must hold only the key it is handed, whatever the runner's
	// environment carries.
	withheld := map[string]bool{}
	for _, name := range credentials.ProviderEnv() {
		withheld[name] = true
	}
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); !withheld[name] {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, keySetChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child did not finish within 60s:\n%s", out)
	}
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "--- SKIP") {
		t.Skipf("child could not withhold locked memory:\n%s", out)
	}
	if !strings.Contains(string(out), "--- PASS: TestKeySetRefusesKeyThatCannotBeSealed") {
		t.Fatalf("child reported no result:\n%s", out)
	}
}

func TestKeySetConfiguresSealableKey(t *testing.T) {
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	response := keySetOverServe(t, a, "jarvis-test-key-0123456789abcdef")
	if !response.OK || string(response.Result) != `{"configured":true}` {
		t.Fatalf("jarvis.key.set = %+v, want configured:true", response)
	}
	secret, err := a.credentials.Resolve("gemini")
	if err != nil || secret.Source() != "workbench-memory" {
		t.Fatalf("Resolve after key.set = source %q, %v; want the workbench key", secret.Source(), err)
	}
}

func keySetUnsealableChild(t *testing.T) {
	limit := map[string]int{"darwin": 6, "linux": 8}[runtime.GOOS] // RLIMIT_MEMLOCK
	var current syscall.Rlimit
	if err := syscall.Getrlimit(limit, &current); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Setrlimit(limit, &syscall.Rlimit{Cur: 0, Max: current.Max}); err != nil {
		t.Fatal(err)
	}
	// A privileged process may lock memory past the limit, and then nothing
	// here can make a seal fail.
	page, err := syscall.Mmap(-1, 0, os.Getpagesize(), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	if syscall.Mlock(page) == nil {
		t.Skip("mlock succeeds with RLIMIT_MEMLOCK=0 in this process")
	}
	if err := syscall.Munmap(page); err != nil {
		t.Fatal(err)
	}

	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	const key = "jarvis-test-key-0123456789abcdef"
	response := keySetOverServe(t, a, key)
	if response.OK {
		t.Fatalf("jarvis.key.set accepted a key that cannot be sealed: %s", response.Result)
	}
	if response.Error == nil || response.Error.Code != "E_JARVIS" {
		t.Fatalf("jarvis.key.set error = %+v, want code E_JARVIS", response.Error)
	}
	if strings.Contains(response.Error.Message, key) {
		t.Fatal("jarvis.key.set error carries the key")
	}
	if !strings.Contains(response.Error.Message, credentials.ErrLockedMemory.Error()) {
		t.Fatalf("jarvis.key.set error = %q, want it to name the locked-memory failure", response.Error.Message)
	}
	if _, err := a.credentials.Resolve("gemini"); !errors.Is(err, credentials.ErrLockedMemory) {
		t.Fatalf("Resolve after the refused key = %v, want ErrLockedMemory", err)
	}
}

// keySetOverServe sends jarvis.key.set through the host plane, as the
// workbench does, and returns its response.
func keySetOverServe(t *testing.T, a *App, key string) serve.Response {
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
	params, err := json.Marshal(map[string]string{"key": key})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		enc := json.NewEncoder(send)
		for _, request := range []serve.Request{
			{ID: "hello", Op: "hello", Params: json.RawMessage(`{"protocol":1,"host":"key-set-test"}`)},
			{ID: "key", Op: "jarvis.key.set", Params: params},
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
			if response.ID != "key" {
				t.Fatalf("unexpected response %+v", response)
			}
			return response
		case <-timer.C:
			t.Fatal("jarvis.key.set did not answer within 20s")
		}
	}
}
