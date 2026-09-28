package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	offline "github.com/bharathvbcr/Jarvis/internal/trace"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/gussetcheck"
	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
	"github.com/bharathvbcr/Manvi/manvi/workflow/catalog"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "jarvis:", err)
		os.Exit(1)
	}
}
func output(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func run() error {
	if len(os.Args) < 2 {
		return errors.New("commands: doctor, discover, compile, replay, drift, trace replay, verify, catalog, codegen, serve, gusset-check")
	}
	command := os.Args[1]
	args := os.Args[2:]
	if command == "trace" {
		if len(args) == 0 || args[0] != "replay" {
			return errors.New("use trace replay")
		}
		command = "trace-replay"
		args = args[1:]
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	root := f.String("root", ".", "Jarvis product root")
	broker := f.String("broker", "", "native broker executable")
	verifier := f.String("verifier", "", "dcverify executable")
	bank := f.String("bank", "", "bank executable")
	capability := f.String("capability", "examples/balance.json", "capability artifact")
	contract := f.String("contract", "scenarios/balance-m1001.contract.json", "independent acceptance contract")
	inputsPath := f.String("inputs", "scenarios/m1001.inputs.json", "typed input JSON")
	tenant := f.String("tenant", "north", "north or south")
	pid := f.Uint("pid", 0, "attach existing bank PID")
	fault := f.String("fault", "none", "seeded bank fault")
	variant := f.String("variant", "none", "bank UI variant (none|renamed-controls)")
	destination := f.String("out", "", "output file")
	register := f.Bool("register", false, "register immutable revision")
	task := f.String("task", "Look up the member balance, extract it as money in USD, and publish a reusable balance capability.", "discovery objective")
	provider := f.String("provider", "gemini", "discovery LLM: gemini (live) or fixture (recorded Manvi mock-wire transcript; evidence labelled not live)")
	fixture := f.String("fixture", "", "path to recorded discovery tool transcript for --provider fixture (default: bundled testdata)")
	tracePath := f.String("trace", "", "offline trace path")
	resumeSession := f.String("resume-session", "", "private saved Gemini journal for continuation")
	assisted := f.Bool("assisted", false, "enable one explicitly requested safe model recovery")
	unattended := f.Bool("unattended", false, "refuse draft/revoked catalog capabilities")
	bundle := f.String("bundle", "", "evidence bundle path")
	runID := f.String("run-id", "", "independently expected run ID")
	sessionID := f.String("session-id", "", "independently expected session ID")
	epoch := f.Uint64("epoch", 1, "independent admission epoch")
	contractSHA := f.String("contract-sha256", "", "independently expected contract hash")
	capabilitySHA := f.String("capability-sha256", "", "independently expected capability hash")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if uint64(*pid) > uint64(^uint32(0)) {
		return errors.New("PID exceeds platform identifier range")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	resolve := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(absRoot, path)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	readInputs := func() (map[string]workflow.Value, error) {
		raw, err := os.ReadFile(resolve(*inputsPath))
		if err != nil {
			return nil, err
		}
		var values map[string]workflow.Value
		err = workflow.DecodeStrict(raw, &values)
		return values, err
	}
	// The engine check needs no product host, broker, or bank either.
	if command == "gusset-check" {
		return gussetCheck(ctx, os.Stdout)
	}
	// Offline reconstruction never creates the product host, broker, or provider.
	if command == "trace-replay" {
		raw, err := os.ReadFile(resolve(*tracePath))
		if err != nil {
			return err
		}
		cap, err := os.ReadFile(resolve(*capability))
		if err != nil {
			return err
		}
		inputs, err := readInputs()
		if err != nil {
			return err
		}
		state, err := offline.Replay(cap, raw, inputs)
		if err != nil {
			return err
		}
		return output(state)
	}
	a, err := app.New(ctx, app.Config{Root: absRoot, Broker: *broker, Verifier: *verifier, Bank: *bank})
	if err != nil {
		return err
	}
	defer a.Close()
	switch command {
	case "serve":
		err := serve.New(os.Stdout, serve.Options{HardRules: true, Modules: []serve.Module{a}}).Serve(ctx, os.Stdin)
		// The policy plane's engine handle, when this build links one. Bounded,
		// so a stuck engine is reported rather than hanging exit.
		return errors.Join(err, gussetcheck.Shutdown(2*time.Second))
	case "doctor":
		return output(a.Doctor(ctx))
	case "compile":
		p, err := a.Compile(*capability)
		if err != nil {
			return err
		}
		if *register {
			e, err := a.Catalog().Put(p)
			if err != nil {
				return err
			}
			return output(e)
		}
		return output(struct {
			SHA256     string              `json:"sha256"`
			Capability workflow.Capability `json:"capability"`
		}{p.Digest(), p.Capability()})
	case "drift":
		report, err := a.Drift(ctx, *capability, *tenant, uint32(*pid))
		if err != nil {
			return err
		}
		return output(report)
	case "catalog":
		entries, err := a.Catalog().List()
		if err != nil {
			return err
		}
		return output(entries)
	case "codegen":
		p, err := a.Compile(*capability)
		if err != nil {
			return err
		}
		source, err := catalog.GenerateGo(p)
		if err != nil {
			return err
		}
		if *destination != "" {
			return os.WriteFile(resolve(*destination), source, 0600)
		}
		_, err = os.Stdout.Write(source)
		return err
	case "verify":
		binary := *verifier
		if binary == "" {
			binary = filepath.Join(absRoot, "build", "dcverify")
		}
		report, err := devcouncil.VerifyEvidence(ctx, devcouncil.EvidenceRequest{Binary: binary, ContractPath: resolve(*contract), BundlePath: resolve(*bundle), ArtifactsRoot: filepath.Dir(resolve(*bundle)), ContractSHA256: *contractSHA, CapabilitySHA256: *capabilitySHA, RunID: *runID, SessionID: *sessionID, Epoch: *epoch})
		if err != nil {
			return err
		}
		if err = output(report); err != nil {
			return err
		}
		if report.Verdict != "passed" {
			return fmt.Errorf("evidence verdict: %s", report.Verdict)
		}
		return nil
	case "replay", "discover":
		inputs, err := readInputs()
		if err != nil {
			return err
		}
		var id string
		if command == "replay" {
			if *unattended {
				if err = a.RequireApprovedForUnattended(*capability); err != nil {
					return err
				}
			}
			id, err = a.Start(app.StartRequest{CapabilityPath: *capability, ContractPath: *contract, Tenant: *tenant, Inputs: inputs, PID: uint32(*pid), Fault: *fault, Variant: *variant, Assisted: *assisted})
		} else {
			id, err = a.Discover(app.DiscoveryRequest{Tenant: *tenant, Task: *task, Inputs: inputs, PID: uint32(*pid), ResumeSession: *resumeSession, Provider: *provider, Fixture: *fixture})
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "run", id)
		lines := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				select {
				case lines <- scanner.Text():
				case <-ctx.Done():
					return
				}
			}
		}()
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		var prompted string
		for {
			select {
			case <-ctx.Done():
				v, _ := a.Get(id)
				_ = a.Control(context.Background(), id, computer.Control{Kind: "cancel", Epoch: v.State.Epoch})
				return ctx.Err()
			case text := <-lines:
				v, err := a.Get(id)
				if err != nil {
					return err
				}
				line := strings.TrimSpace(text)
				kind := strings.ToLower(line)
				if kind == "assist" {
					if err = a.Assist(id); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
					continue
				}
				if kind == "takeover" {
					if err = a.Takeover(ctx, id); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
					continue
				}
				if kind == "handback" {
					if err = a.Handback(ctx, id); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
					continue
				}
				if strings.HasPrefix(kind, "act ") {
					if err = a.ActHuman(ctx, id, strings.TrimSpace(line[3:])); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
					continue
				}
				if kind == "approve" || kind == "deny" || kind == "cancel" || kind == "pause" || kind == "resume" || kind == "refresh" || kind == "focus" {
					if err = a.Control(ctx, id, computer.Control{Kind: kind, Epoch: v.State.Epoch, ActionID: v.PendingActionID, ObservationID: v.State.Observation.ID}); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				}
			case <-ticker.C:
				v, err := a.Get(id)
				if err != nil {
					return err
				}
				if v.Finished {
					if err = output(v); err != nil {
						return err
					}
					if v.Error != "" {
						return errors.New(v.Error)
					}
					if v.Report != nil && v.Report.Verdict != "passed" {
						return fmt.Errorf("independent verification: %s", v.Report.Verdict)
					}
					return nil
				}
				key := string(v.State.Phase) + v.PendingActionID + v.State.Observation.ID
				if (v.State.Phase == workflow.AwaitingApproval || v.State.Phase == workflow.Paused) && key != prompted {
					prompted = key
					fmt.Fprintf(os.Stderr, "%s: action=%s observation=%s epoch=%d controller=%s reason=%s\nType approve, deny, cancel, pause, takeover, act <kind name>, handback, refresh, focus, or resume.\n", v.State.Phase, v.PendingActionID, v.State.Observation.ID, v.State.Epoch, v.Controller, v.State.Reason)
				}
			}
		}
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}
