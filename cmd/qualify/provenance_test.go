package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestArtifactBindingDetectsContentAndSymlinkRetargeting(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	if err := os.WriteFile(first, []byte("original"), 0700); err != nil {
		t.Fatal(err)
	}
	binding, err := bindFile(first)
	if err != nil {
		t.Fatal(err)
	}
	p := provenance{Files: map[string]fileBinding{"bank": binding}}
	if err = p.check(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(first, []byte("modified"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = p.check(); err == nil {
		t.Fatal("changed binary retained old identity")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err = os.WriteFile(second, []byte("other"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "current")
	if err = os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	binding, err = bindFile(link)
	if err != nil {
		t.Fatal(err)
	}
	p.Files["bank"] = binding
	if err = os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if err = p.check(); err == nil {
		t.Fatal("retargeted executable symlink retained old identity")
	}
}
