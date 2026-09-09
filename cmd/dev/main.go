// dev is the product's reproducible build/check entry point. It invokes pinned
// upstream workspaces without adding CGO or another first-party language.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type lock struct {
	SchemaVersion int    `json:"schema_version"`
	Manvi         source `json:"manvi"`
	DevCouncil    source `json:"devcouncil"`
}
type source struct {
	URL      string `json:"url"`
	Revision string `json:"revision"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dev:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("commands: bootstrap, workspace, build, test, qualify, check-pins, bundle-sources, package")
	}
	action := os.Args[1]
	f := flag.NewFlagSet(action, flag.ContinueOnError)
	manvi := f.String("manvi", os.Getenv("JARVIS_MANVI_SOURCE"), "Manvi checkout root")
	dc := f.String("devcouncil", os.Getenv("JARVIS_DEVCOUNCIL_SOURCE"), "DevCouncil checkout root")
	release := f.Bool("release", false, "optimized Rust build")
	bundles := f.String("bundles", "build/upstream", "directory containing portable upstream Git bundles")
	if err := f.Parse(os.Args[2:]); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if *manvi == "" {
		*manvi = filepath.Join(root, ".local", "upstream", "Manvi")
	}
	if *dc == "" {
		*dc = filepath.Join(root, ".local", "upstream", "DevCouncil")
	}
	*manvi, err = filepath.Abs(*manvi)
	if err != nil {
		return err
	}
	*dc, err = filepath.Abs(*dc)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	command := func(dir, name string, args ...string) error {
		fmt.Fprintln(os.Stderr, name, strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.WaitDelay = 2 * time.Second
		return cmd.Run()
	}
	workspace := func() error {
		if _, err := os.Stat(filepath.Join(*manvi, "manvi", "go.mod")); err != nil {
			return fmt.Errorf("Manvi checkout missing: %w", err)
		}
		text := fmt.Sprintf("go 1.26.6\nuse (\n .\n %q\n)\nreplace github.com/bharathvbcr/Manvi/manvi v0.0.0 => %q\n", filepath.Join(*manvi, "manvi"), filepath.Join(*manvi, "manvi"))
		return os.WriteFile(filepath.Join(root, "go.work"), []byte(text), 0600)
	}
	switch action {
	case "bootstrap":
		if err := bootstrap(ctx, *manvi, *dc, *bundles); err != nil {
			return err
		}
		return workspace()
	case "bundle-sources":
		return bundleSources(ctx, *manvi, *dc, *bundles)
	case "package":
		return packageBuild(root)
	case "workspace":
		return workspace()
	case "check-pins":
		return checkPins(ctx, *manvi, *dc)
	case "build":
		if err := workspace(); err != nil {
			return err
		}
		if err := os.MkdirAll("build", 0755); err != nil {
			return err
		}
		flavor := "debug"
		flags := []string{"build", "--locked"}
		if *release {
			flags = append(flags, "--release")
			flavor = "release"
		}
		for _, dir := range []string{filepath.Join(*manvi, "native"), filepath.Join(root, "desktop")} {
			args := append(append([]string(nil), flags...), "--target-dir", filepath.Join(dir, "target"))
			if err := command(dir, "cargo", args...); err != nil {
				return err
			}
		}
		dcargs := append(append([]string(nil), flags...), "-p", "dc-verify", "--target-dir", filepath.Join(*dc, "rust", "target"))
		if err := command(filepath.Join(*dc, "rust"), "cargo", dcargs...); err != nil {
			return err
		}
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		for name, path := range map[string]string{"manvi-desktop": filepath.Join(*manvi, "native", "target", flavor, "manvi-desktop"+suffix), "dcverify": filepath.Join(*dc, "rust", "target", flavor, "dcverify"+suffix), "jarvis-bank": filepath.Join(root, "desktop", "target", flavor, "jarvis-bank"+suffix), "jarvis-workbench": filepath.Join(root, "desktop", "target", flavor, "jarvis-workbench"+suffix)} {
			if err := copyBinary(path, filepath.Join(root, "build", name+suffix)); err != nil {
				return err
			}
		}
		if err := command(root, "go", "build", "-trimpath", "-o", filepath.Join("build", "jarvis"+suffix), "./cmd/jarvis"); err != nil {
			return err
		}
		if runtime.GOOS == "darwin" {
			return bundleMacApp(root)
		}
		return nil
	case "test":
		if err := workspace(); err != nil {
			return err
		}
		for _, item := range []struct {
			dir, name string
			args      []string
		}{{root, "go", []string{"test", "./..."}}, {filepath.Join(*manvi, "manvi"), "go", []string{"test", "./workflow/...", "./computer/...", "./llm/budget/...", "./session", "./tools", "./llm/replay", "./llm/gemini", "./llm/transport", "./agent", "./serve"}}, {filepath.Join(root, "desktop"), "cargo", []string{"test", "--locked", "--workspace"}}, {filepath.Join(*manvi, "native"), "cargo", []string{"test", "--locked", "--workspace"}}, {filepath.Join(*dc, "rust"), "cargo", []string{"test", "--locked", "-p", "dc-evidence", "-p", "dc-verify"}}} {
			if err := command(item.dir, item.name, item.args...); err != nil {
				return err
			}
		}
		return command(root, "go", "run", "./cmd/qualify", "--check-boundaries")
	case "qualify":
		return command(root, "go", append([]string{"run", "./cmd/qualify"}, f.Args()...)...)
	default:
		return fmt.Errorf("unknown command %q", action)
	}
}
func copyBinary(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("binary source must be regular")
	}
	if target, err := os.Lstat(dst); err == nil {
		if !target.Mode().IsRegular() || os.SameFile(info, target) {
			return errors.New("binary destination must be a different regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	opened, err := in.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return errors.New("binary source changed during open")
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".jarvis-binary-*")
	if err != nil {
		return err
	}
	defer func() { _ = out.Close(); _ = os.Remove(out.Name()) }()
	if err := out.Chmod(0755); err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(in, info.Size()+1))
	if err != nil {
		return err
	}
	if n != info.Size() {
		return errors.New("binary source size changed during copy")
	}
	if err = out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(out.Name(), dst)
}
