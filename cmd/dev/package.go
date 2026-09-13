package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func readPins() (lock, error) {
	var pins lock
	raw, err := os.ReadFile("upstream.lock.json")
	if err != nil {
		return pins, err
	}
	if err = json.Unmarshal(raw, &pins); err != nil {
		return pins, err
	}
	if pins.SchemaVersion != 1 {
		return pins, errors.New("unsupported source pin schema")
	}
	for _, s := range []source{pins.Manvi, pins.DevCouncil} {
		hash, err := hex.DecodeString(s.Revision)
		if err != nil || len(hash) != 20 || !strings.HasPrefix(s.URL, "https://github.com/bharathvbcr/") {
			return pins, errors.New("invalid source revision or origin")
		}
	}
	return pins, nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// goModuleRevision reports the Manvi commit that go.mod resolves to. The Go
// host builds from this version while the Rust native side builds from the
// pinned checkout, so a divergence silently ships two different Manvi revisions
// in one artifact. Returns the commit for a pseudo-version directly and
// resolves a tagged version against the checkout.
func goModuleRevision(ctx context.Context, manvi string) (string, error) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	const module = "github.com/bharathvbcr/Manvi/manvi"
	version := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		// Accept both the single-line "require mod ver" form and a require block.
		if len(fields) >= 3 && fields[0] == "require" {
			fields = fields[1:]
		}
		if len(fields) >= 2 && fields[0] == module {
			version = fields[1]
			break
		}
	}
	if version == "" {
		return "", fmt.Errorf("go.mod does not require %s", module)
	}
	// A pseudo-version ends in -<12 hex digits of the commit>.
	if i := strings.LastIndex(version, "-"); i >= 0 && len(version)-i-1 == 12 {
		if _, err := hex.DecodeString(version[i+1:]); err == nil {
			return version[i+1:], nil
		}
	}
	return git(ctx, manvi, "rev-parse", version+"^{commit}")
}

// checkGoModuleMatchesPin fails when go.mod and upstream.lock.json disagree.
func checkGoModuleMatchesPin(ctx context.Context, manvi string, pin source) error {
	rev, err := goModuleRevision(ctx, manvi)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(pin.Revision, rev) && rev != pin.Revision {
		return fmt.Errorf("go.mod builds Manvi %s but upstream.lock.json pins %s; the Go host and the native workspace would not share a revision", rev, pin.Revision)
	}
	return nil
}

func checkPins(ctx context.Context, manvi, dc string) error {
	pins, err := readPins()
	if err != nil {
		return err
	}
	for _, p := range []struct {
		path string
		pin  source
	}{{manvi, pins.Manvi}, {dc, pins.DevCouncil}} {
		head, err := git(ctx, p.path, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		if head != p.pin.Revision {
			return fmt.Errorf("upstream HEAD differs from pin: %s", p.path)
		}
		dirty, err := git(ctx, p.path, "status", "--porcelain", "--untracked-files=normal")
		if err != nil {
			return err
		}
		if dirty != "" {
			return fmt.Errorf("upstream contains changes outside pinned revision: %s", p.path)
		}
	}
	return checkGoModuleMatchesPin(ctx, manvi, pins.Manvi)
}

func bootstrap(ctx context.Context, manvi, dc, bundles string) error {
	pins, err := readPins()
	if err != nil {
		return err
	}
	for _, p := range []struct {
		name, path string
		pin        source
	}{{"Manvi", manvi, pins.Manvi}, {"DevCouncil", dc, pins.DevCouncil}} {
		if _, err := os.Stat(p.path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p.path), 0700); err != nil {
			return err
		}
		tmp, err := os.MkdirTemp(filepath.Dir(p.path), ".jarvis-source-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		origin := p.pin.URL
		bundle, err := filepath.Abs(filepath.Join(bundles, p.name+".bundle"))
		if err != nil {
			return err
		}
		if info, err := os.Lstat(bundle); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("source bundle must be a regular file")
			}
			origin = bundle
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := git(ctx, ".", "clone", "--no-checkout", origin, tmp); err != nil {
			return err
		}
		if _, err := git(ctx, tmp, "checkout", "--detach", p.pin.Revision); err != nil {
			return err
		}
		if _, err := git(ctx, tmp, "remote", "set-url", "origin", p.pin.URL); err != nil {
			return err
		}
		if err := os.Rename(tmp, p.path); err != nil {
			return err
		}
	}
	return checkPins(ctx, manvi, dc)
}

func bundleSources(ctx context.Context, manvi, dc, bundles string) error {
	if err := checkPins(ctx, manvi, dc); err != nil {
		return err
	}
	if err := os.MkdirAll(bundles, 0700); err != nil {
		return err
	}
	for _, p := range []struct{ name, path string }{{"Manvi", manvi}, {"DevCouncil", dc}} {
		out, err := filepath.Abs(filepath.Join(bundles, p.name+".bundle"))
		if err != nil {
			return err
		}
		if _, err := os.Lstat(out); err == nil {
			return fmt.Errorf("bundle already exists: %s", out)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := git(ctx, p.path, "bundle", "create", out, "HEAD"); err != nil {
			return err
		}
		if _, err := git(ctx, p.path, "bundle", "verify", out); err != nil {
			return err
		}
	}
	return nil
}

func bundleMacApp(root string) error {
	contents := filepath.Join(root, "build", "JarvisWorkbench.app", "Contents")
	if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0755); err != nil {
		return err
	}
	if err := copyBinary(filepath.Join(root, "build", "jarvis-workbench"), filepath.Join(contents, "MacOS", "jarvis-workbench")); err != nil {
		return err
	}
	plist, err := os.ReadFile(filepath.Join(root, "desktop", "packaging", "macos", "Info.plist"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contents, "Info.plist"), plist, 0644)
}

type packageFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func packageBuild(root string) (err error) {
	name := "jarvis-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	path := filepath.Join(root, "build", name)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("package already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(root, "build"), ".package-*.zip")
	if err != nil {
		return err
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	z := zip.NewWriter(tmp)
	files := []packageFile{}
	add := func(path, name string) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package input must be regular: %s", path)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name, header.Method = filepath.ToSlash(name), zip.Deflate
		writer, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		opened, err := in.Stat()
		if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			_ = in.Close()
			return errors.Join(err, errors.New("package input changed during open"))
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(writer, h), io.LimitReader(in, info.Size()+1))
		if err := errors.Join(copyErr, in.Close()); err != nil {
			return err
		}
		if n != info.Size() {
			return errors.New("package input size changed during copy")
		}
		files = append(files, packageFile{header.Name, hex.EncodeToString(h.Sum(nil)), n})
		return nil
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, name := range []string{"jarvis", "jarvis-bank", "jarvis-workbench", "manvi-desktop", "dcverify"} {
		if err := add(filepath.Join(root, "build", name+suffix), filepath.Join("build", name+suffix)); err != nil {
			return err
		}
	}
	for _, name := range []string{"README.md", "REPORT.md", "go.mod", "upstream.lock.json"} {
		if err := add(filepath.Join(root, name), name); err != nil {
			return err
		}
	}
	for _, dir := range []string{"cmd", "internal", "examples", "profiles", "scenarios", "docs", "tests", "desktop/crates", "desktop/packaging", "build/upstream"} {
		if err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == filepath.Join(root, "tests", "native-platform", "local") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			return add(path, rel)
		}); err != nil {
			return err
		}
	}
	for _, name := range []string{"desktop/Cargo.toml", "desktop/Cargo.lock", "desktop/README.md", "desktop/WORKBENCH.md"} {
		if err := add(filepath.Join(root, name), name); err != nil {
			return err
		}
	}
	// Only deliberately curated example bundles and top-level qualification
	// reports are exportable. Raw per-run evidence/private is never traversed.
	evidence := filepath.Join(root, "evidence")
	if entries, err := os.ReadDir(evidence); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "README.md") {
				if err := add(filepath.Join(evidence, entry.Name()), filepath.Join("evidence", entry.Name())); err != nil {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	curated := filepath.Join(evidence, "examples")
	if info, err := os.Lstat(curated); err == nil {
		if !info.IsDir() {
			return errors.New("curated evidence must be a directory")
		}
		if err := filepath.WalkDir(curated, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			return add(path, rel)
		}); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if runtime.GOOS == "darwin" {
		for _, name := range []string{"Contents/Info.plist", "Contents/MacOS/jarvis-workbench"} {
			rel := filepath.Join("build", "JarvisWorkbench.app", name)
			if err := add(filepath.Join(root, rel), rel); err != nil {
				return err
			}
		}
	}
	w, err := z.Create("artifact-manifest.json")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(w).Encode(files); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Link(tmp.Name(), path)
}
