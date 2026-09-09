package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type fileBinding struct {
	Path         string `json:"path"`
	ResolvedPath string `json:"resolved_path"`
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
}
type provenance struct {
	Files          map[string]fileBinding `json:"files"`
	SourceRevision string                 `json:"source_revision,omitempty"`
	SourceStatus   string                 `json:"source_status"`
}

func bindFile(path string) (fileBinding, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fileBinding{}, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fileBinding{}, err
	}
	f, err := os.Open(resolved)
	if err != nil {
		return fileBinding{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return fileBinding{}, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > 512<<20 {
		return fileBinding{}, errors.New("qualification artifact must be a nonempty regular file of at most 512 MiB")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(f, (512<<20)+1))
	if err != nil {
		return fileBinding{}, err
	}
	after, err := f.Stat()
	if err != nil {
		return fileBinding{}, err
	}
	if size != before.Size() || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return fileBinding{}, errors.New("qualification artifact changed while hashing")
	}
	return fileBinding{absolute, resolved, hex.EncodeToString(hash.Sum(nil)), size}, nil
}
func bindCampaign(ctx context.Context, root, broker, bank, verifier, scenario string) (provenance, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return provenance{}, err
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if broker == "" {
		broker = filepath.Join(root, "build", "manvi-desktop"+suffix)
	}
	if bank == "" {
		bank = filepath.Join(root, "build", "jarvis-bank"+suffix)
	}
	if verifier == "" {
		verifier = filepath.Join(root, "build", "dcverify"+suffix)
	}
	capability, contract := "examples/balance.json", "scenarios/balance-m1001.contract.json"
	if scenario != "balance" {
		capability, contract = "examples/create-subaccount.json", "scenarios/subaccount.contract.json"
	}
	p := provenance{Files: map[string]fileBinding{}, SourceStatus: "unavailable"}
	paths := map[string]string{"capability": filepath.Join(root, capability), "contract": filepath.Join(root, contract), "broker": broker, "bank": bank, "verifier": verifier}
	for _, name := range []string{"broker", "bank", "verifier"} {
		resolved, err := exec.LookPath(paths[name])
		if err != nil {
			return provenance{}, fmt.Errorf("resolve %s executable: %w", name, err)
		}
		paths[name] = resolved
	}
	if self, err := os.Executable(); err == nil {
		paths["qualifier"] = self
	}
	for name, path := range paths {
		binding, err := bindFile(path)
		if err != nil {
			return provenance{}, fmt.Errorf("bind %s: %w", name, err)
		}
		p.Files[name] = binding
	}
	if head, err := gitText(ctx, root, "rev-parse", "HEAD"); err == nil && len(strings.TrimSpace(head)) == 40 {
		p.SourceRevision = strings.TrimSpace(head)
	}
	if status, err := gitText(ctx, root, "status", "--porcelain", "--untracked-files=normal"); err == nil {
		p.SourceStatus = "clean"
		if strings.TrimSpace(status) != "" {
			p.SourceStatus = "dirty"
		}
	}
	return p, nil
}
func (p provenance) check() error {
	for name, want := range p.Files {
		got, err := bindFile(want.Path)
		if err != nil {
			return fmt.Errorf("recheck %s: %w", name, err)
		}
		if got != want {
			return fmt.Errorf("qualification artifact %s changed after campaign admission", name)
		}
	}
	return nil
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(raw []byte) (int, error) {
	if b.Len()+len(raw) > 65536 {
		return 0, errors.New("git provenance output exceeds 64 KiB")
	}
	return b.Buffer.Write(raw)
}
func gitText(parent context.Context, root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	var output limitedOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return output.String(), nil
}
