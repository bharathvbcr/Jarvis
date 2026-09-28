package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFixture(t *testing.T, root, path string, data []byte) {
	t.Helper()
	path = filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func sourceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func packageFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"README.md", "REPORT.md", "go.mod", "upstream.lock.json", "cmd/dev/main.go", "internal/app/app.go", "examples/balance.json", "profiles/north.json", "scenarios/balance.contract.json", "docs/IMPLEMENTATION.md", "tests/native-platform/test.go", "desktop/crates/bank-demo/Cargo.toml", "desktop/crates/workbench/Cargo.toml", "desktop/crates/desktop-ui/Cargo.toml", "desktop/crates/desktop-ui/src/lib.rs", "desktop/packaging/macos/Info.plist", "desktop/Cargo.toml", "desktop/Cargo.lock", "desktop/README.md", "desktop/WORKBENCH.md", "build/upstream/Manvi.bundle", "build/upstream/DevCouncil.bundle", "evidence/report.json", "evidence/examples/reviewed/frame.png", "evidence/private/secret.json", "evidence/unreviewed/frame.png"} {
		writeFixture(t, root, path, []byte(path))
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, name := range []string{"jarvis", "jarvis-bank", "jarvis-workbench", "manvi-desktop", "dcverify"} {
		writeFixture(t, root, "build/"+name+suffix, []byte(name))
	}
	if runtime.GOOS == "darwin" {
		for _, path := range []string{"build/JarvisWorkbench.app/Contents/Info.plist", "build/JarvisWorkbench.app/Contents/MacOS/jarvis-workbench"} {
			writeFixture(t, root, path, []byte(path))
		}
	}
	return root
}

func TestPackageIncludesCuratedEvidenceAndCompleteRustWorkspace(t *testing.T) {
	root := packageFixture(t)
	writeFixture(t, root, "tests/native-platform/local/private-vm/disk.img", []byte("private guest state"))
	if err := packageBuild(root); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(filepath.Join(root, "build", "jarvis-"+runtime.GOOS+"-"+runtime.GOARCH+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := map[string][]byte{}
	for _, file := range reader.File {
		in, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(in)
		closeErr := in.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		entries[file.Name] = data
	}
	for _, path := range []string{"evidence/report.json", "evidence/examples/reviewed/frame.png", "desktop/Cargo.toml", "desktop/Cargo.lock", "desktop/crates/desktop-ui/Cargo.toml", "desktop/crates/desktop-ui/src/lib.rs"} {
		if _, ok := entries[path]; !ok {
			t.Errorf("required package entry missing: %s", path)
		}
	}
	for path := range entries {
		if strings.HasPrefix(path, "tests/native-platform/local/") || strings.HasPrefix(path, "evidence/private/") || strings.HasPrefix(path, "evidence/unreviewed/") {
			t.Errorf("unselected evidence packaged: %s", path)
		}
	}
	var manifest []packageFile
	if err := json.Unmarshal(entries["artifact-manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != len(entries)-1 {
		t.Fatal("manifest inventory does not match ZIP")
	}
	for _, item := range manifest {
		data, ok := entries[item.Path]
		sum := sha256.Sum256(data)
		if !ok || int64(len(data)) != item.Bytes || hex.EncodeToString(sum[:]) != item.SHA256 {
			t.Fatalf("manifest mismatch: %+v", item)
		}
	}
}

func TestCopyBinaryRefusesSymlinkAndLeavesTargetUnchanged(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	dst := filepath.Join(root, "destination")
	writeFixture(t, root, "source", []byte("new binary"))
	writeFixture(t, root, "target", []byte("preserve me"))
	if err := os.Symlink(target, dst); err != nil {
		t.Fatal(err)
	}
	if err := copyBinary(src, dst); err == nil {
		t.Error("symlink destination accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "preserve me" {
		t.Fatalf("symlink target overwritten: %q %v", data, err)
	}
}

func TestPackageDoesNotReplaceDestinationCreatedDuringAssembly(t *testing.T) {
	root := packageFixture(t)
	writeFixture(t, root, "cmd/large-fixture.data", bytes.Repeat([]byte("large fixture input\n"), 2<<20))
	destination := filepath.Join(root, "build", "jarvis-"+runtime.GOOS+"-"+runtime.GOARCH+".zip")
	done := make(chan error, 1)
	go func() { done <- packageBuild(root) }()
	deadline := time.After(5 * time.Second)
	for {
		staged, err := filepath.Glob(filepath.Join(root, "build", ".package-*.zip"))
		if err != nil {
			t.Fatal(err)
		}
		if len(staged) > 0 {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("assembly ended before publication test: %v", err)
		case <-deadline:
			t.Fatal("assembly did not stage archive")
		case <-time.After(time.Millisecond):
		}
	}
	if err := os.WriteFile(destination, []byte("concurrent reservation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Error("package replaced a destination created during assembly")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "concurrent reservation" {
		t.Fatal("concurrent destination was overwritten")
	}
}

func TestBootstrapRunsOfflineWithoutWorkspaceOrUpstreamModules(t *testing.T) {
	root := t.TempDir()
	original := sourceRoot(t)
	files, err := os.ReadDir(filepath.Join(original, "cmd/dev"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".go") && !strings.HasSuffix(file.Name(), "_test.go") {
			raw, err := os.ReadFile(filepath.Join(original, "cmd/dev", file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, "cmd/dev/"+file.Name(), raw)
		}
	}
	mod, err := os.ReadFile(filepath.Join(original, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "go.mod", mod)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pins := lock{SchemaVersion: 1}
	// Dependency order: each upstream pins the ones before it, and check-pins
	// requires those pins to agree with the lock.
	for _, item := range []struct {
		name string
		pin  *source
	}{{"gusset", &pins.Gusset}, {"DevCouncil", &pins.DevCouncil}, {"Manvi", &pins.Manvi}} {
		repo := t.TempDir()
		switch item.name {
		case "Manvi":
			writeFixture(t, repo, "manvi/go.mod", []byte("module github.com/bharathvbcr/Manvi/manvi\n\ngo 1.26.6\n"))
			writeFixture(t, repo, "scripts/module-pins.txt", []byte(
				"DevCouncil\thttps://github.com/bharathvbcr/DevCouncil.git\t"+pins.DevCouncil.Revision+
					"\ngusset\thttps://github.com/bharathvbcr/gusset.git\t"+pins.Gusset.Revision+"\n"))
		case "DevCouncil":
			writeFixture(t, repo, ".github/actions/setup-gusset/action.yml", []byte("        ref: "+pins.Gusset.Revision+"\n"))
		default:
			writeFixture(t, repo, "README.md", []byte("fixture"))
		}
		for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"-c", "user.name=Bootstrap test", "-c", "user.email=bootstrap@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture"}} {
			if _, err := git(ctx, repo, args...); err != nil {
				t.Fatal(err)
			}
		}
		revision, err := git(ctx, repo, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		*item.pin = source{URL: "https://github.com/bharathvbcr/" + item.name + ".git", Revision: revision}
		bundle := filepath.Join(root, "build", "upstream", item.name+".bundle")
		if err := os.MkdirAll(filepath.Dir(bundle), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, repo, "bundle", "create", bundle, "HEAD"); err != nil {
			t.Fatal(err)
		}
	}
	// The fixture pins freshly created repositories, so its go.mod has to name
	// the same Manvi revision. Copying the product's go.mod verbatim would leave
	// the pair inconsistent and check-pins would rightly refuse to bootstrap it.
	lines := strings.Split(string(mod), "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "require" && fields[1] == "github.com/bharathvbcr/Manvi/manvi" {
			lines[i] = "require github.com/bharathvbcr/Manvi/manvi v0.0.0-00010101000000-" + pins.Manvi.Revision[:12]
		}
	}
	writeFixture(t, root, "go.mod", []byte(strings.Join(lines, "\n")))
	raw, err := json.Marshal(pins)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "upstream.lock.json", raw)
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/dev", "bootstrap")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh offline bootstrap failed: %v\n%s", err, output)
	}
	work, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil || !bytes.Contains(work, []byte(filepath.Join(root, ".local", "upstream", "Manvi", "manvi"))) {
		t.Fatalf("workspace missing or wrong: %q %v", work, err)
	}
	for _, item := range []struct {
		name string
		pin  source
	}{{"Manvi", pins.Manvi}, {"DevCouncil", pins.DevCouncil}, {"gusset", pins.Gusset}} {
		head, err := git(ctx, filepath.Join(root, ".local", "upstream", item.name), "rev-parse", "HEAD")
		if err != nil || head != item.pin.Revision {
			t.Fatalf("wrong restored pin %s: %s %v", item.name, head, err)
		}
	}
}

func TestBuildFindsBinariesWithInheritedCargoTargetDirectory(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("Cargo unavailable; Rust build integration not exercised")
	}
	root := t.TempDir()
	original := sourceRoot(t)
	files, err := os.ReadDir(filepath.Join(original, "cmd/dev"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".go") && !strings.HasSuffix(file.Name(), "_test.go") {
			raw, err := os.ReadFile(filepath.Join(original, "cmd/dev", file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			writeFixture(t, root, "cmd/dev/"+file.Name(), raw)
		}
	}
	writeFixture(t, root, "go.mod", []byte("module github.com/bharathvbcr/Jarvis\n\ngo 1.26.6\n"))
	writeFixture(t, root, "cmd/jarvis/main.go", []byte("package main\nfunc main() {}\n"))
	writeFixture(t, root, "desktop/packaging/macos/Info.plist", []byte("<plist/>"))
	manvi, dc := filepath.Join(root, "Manvi"), filepath.Join(root, "DevCouncil")
	writeFixture(t, manvi, "manvi/go.mod", []byte("module github.com/bharathvbcr/Manvi/manvi\n\ngo 1.26.6\n"))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, item := range []struct {
		dir, name string
		bins      []string
	}{{filepath.Join(manvi, "native"), "native-fixture", []string{"manvi-desktop"}}, {filepath.Join(root, "desktop"), "desktop-fixture", []string{"jarvis-bank", "jarvis-workbench"}}, {filepath.Join(dc, "rust"), "dc-verify", []string{"dcverify"}}} {
		manifest := "[package]\nname = \"" + item.name + "\"\nversion = \"0.1.0\"\nedition = \"2024\"\n[workspace]\n"
		for _, bin := range item.bins {
			manifest += "[[bin]]\nname = \"" + bin + "\"\npath = \"src/" + bin + ".rs\"\n"
			writeFixture(t, item.dir, "src/"+bin+".rs", []byte("fn main() {}\n"))
		}
		writeFixture(t, item.dir, "Cargo.toml", []byte(manifest))
		cmd := exec.CommandContext(ctx, "cargo", "generate-lockfile", "--offline")
		cmd.Dir = item.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture Cargo.lock: %v\n%s", err, out)
		}
	}
	// --engine=false: the fixture has no gusset checkout and a stub host.
	// No unit test covers the linked build: it needs real Manvi, DevCouncil
	// and gusset checkouts and a Rust toolchain. `dev build` proves it on
	// every run instead, by running the built host's gusset-check and failing
	// the build if it does not pass.
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/dev", "build", "--engine=false", "--manvi", manvi, "--devcouncil", dc)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "CARGO_NET_OFFLINE=true", "CARGO_TARGET_DIR="+filepath.Join(root, "redirected-target"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build lost Cargo artifacts with inherited target directory: %v\n%s", err, output)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, bin := range []string{"jarvis", "jarvis-bank", "jarvis-workbench", "manvi-desktop", "dcverify"} {
		info, err := os.Stat(filepath.Join(root, "build", bin+suffix))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("missing built fixture %s: %v", bin, err)
		}
	}
}

// The Go host builds Manvi from go.mod while the native workspace builds from
// the pinned checkout. Nothing compared the two, so they drifted onto different
// revisions and shipped a Go host and a Rust broker from different sources in
// one artifact. check-pins must refuse that.
func TestCheckPinsRefusesAGoModuleThatDisagreesWithThePin(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	pinned := "4818dc2081a53240eb8f4bde48026ad8d704e351"
	write := func(version string) {
		if err := os.WriteFile("go.mod", []byte(
			"module example.com/host\n\ngo 1.26.6\n\nrequire github.com/bharathvbcr/Manvi/manvi "+version+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pin := source{URL: "https://github.com/bharathvbcr/Manvi.git", Revision: pinned}

	write("v0.0.0-20260910143105-adeb253a76f2")
	err = checkGoModuleMatchesPin(context.Background(), dir, pin)
	if err == nil {
		t.Fatal("divergent go.mod accepted")
	}
	if !strings.Contains(err.Error(), "adeb253a76f2") || !strings.Contains(err.Error(), pinned) {
		t.Fatalf("error should name both revisions: %v", err)
	}

	write("v0.0.0-20260910143105-" + pinned[:12])
	if err := checkGoModuleMatchesPin(context.Background(), dir, pin); err != nil {
		t.Fatalf("matching pseudo-version rejected: %v", err)
	}

	if err := os.WriteFile("go.mod", []byte("module example.com/host\n\ngo 1.26.6\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkGoModuleMatchesPin(context.Background(), dir, pin); err == nil {
		t.Fatal("go.mod without the module accepted")
	}
}
