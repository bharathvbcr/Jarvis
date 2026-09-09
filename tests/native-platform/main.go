// Native platform preparation and qualification. Uses only Go's standard library
// and the documented UTM and hdiutil tools; it never operates host application UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "native-platform:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("commands: prepare-linux")
	}
	switch args[0] {
	case "prepare-linux":
		f := flag.NewFlagSet(args[0], flag.ContinueOnError)
		out := f.String("out", "tests/native-platform/local/Jarvis Linux X11.utm", "new UTM bundle; existing bundle is never overwritten")
		archive := f.String("archive", "", "already downloaded pinned Ubuntu archive")
		register := f.Bool("register", false, "register the new bundle in UTM without starting it")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return errors.New("unexpected argument")
		}
		bounded, cancel := context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
		abs, err := filepath.Abs(*out)
		if err != nil {
			return err
		}
		return prepareLinux(bounded, abs, *archive, *register)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
