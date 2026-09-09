package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type result struct {
	RunID                    string         `json:"run_id"`
	Tenant                   string         `json:"tenant"`
	Index                    int            `json:"index"`
	Scenario                 string         `json:"scenario"`
	Attempted                bool           `json:"attempted"`
	Phase                    workflow.Phase `json:"phase"`
	Verdict                  string         `json:"verdict"`
	Outcome                  string         `json:"outcome"`
	LatencyMillis            int64          `json:"latency_millis"`
	ObservationRetries       int            `json:"observation_retries"`
	InputRetries             int            `json:"input_retries"`
	Error                    string         `json:"error,omitempty"`
	EvidenceDir              string         `json:"evidence_dir"`
	Oracle                   string         `json:"oracle"`
	OracleVerified           bool           `json:"oracle_verified"`
	TaskOracleError          string         `json:"task_oracle_error,omitempty"`
	UIAcknowledgmentObserved bool           `json:"ui_acknowledgment_observed"`
	ApprovalDenied           bool           `json:"approval_denied"`
	ApprovedActionIDs        []string       `json:"approved_action_ids"`
	FaultReached             bool           `json:"fault_reached"`
	ExpectedBehaviorVerified bool           `json:"expected_behavior_verified"`
}
type tenantMetrics struct {
	Requested    int `json:"requested"`
	Attempted    int `json:"attempted"`
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Blocked      int `json:"blocked"`
	FaultReached int `json:"fault_reached"`
}
type report struct {
	SchemaVersion          int                      `json:"schema_version"`
	Platform               string                   `json:"platform"`
	Architecture           string                   `json:"architecture"`
	DesktopSession         string                   `json:"desktop_session"`
	StartedAt              string                   `json:"started_at"`
	Mode                   string                   `json:"mode"`
	Scenario               string                   `json:"scenario"`
	RequestedPerTenant     int                      `json:"requested_per_tenant"`
	Tenants                []string                 `json:"tenants"`
	Results                []result                 `json:"results"`
	ByTenant               map[string]tenantMetrics `json:"by_tenant"`
	MatrixComplete         bool                     `json:"matrix_complete"`
	Qualified              bool                     `json:"qualified"`
	Passed                 int                      `json:"passed"`
	Failed                 int                      `json:"failed"`
	Blocked                int                      `json:"blocked"`
	FirstAttemptSuccesses  int                      `json:"first_attempt_successes"`
	ModelRequests          int                      `json:"model_requests"`
	CostNanoUSD            int64                    `json:"cost_nano_usd"`
	OneSided95LowerBound   float64                  `json:"one_sided_95_lower_bound"`
	IndependenceAssumed    bool                     `json:"independence_assumed"`
	Interactive            bool                     `json:"interactive"`
	ExpectedBehaviorPasses int                      `json:"expected_behavior_passes"`
	CampaignPassed         bool                     `json:"campaign_passed"`
	Provenance             provenance               `json:"provenance"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (returnErr error) {
	root := flag.String("root", ".", "product root")
	broker := flag.String("broker", "", "broker executable")
	verifier := flag.String("verifier", "", "verifier executable")
	bank := flag.String("bank", "", "bank executable")
	count := flag.Int("n", 20, "runs per tenant, 1..100")
	tenants := flag.String("tenants", "north,south", "unique tenant list")
	out := flag.String("out", "evidence/platform-stability.json", "new campaign report; existing path is refused")
	boundaries := flag.Bool("check-boundaries", false, "check pure replay dependency boundary only")
	scenario := flag.String("scenario", "balance", "balance, creation, denial, or injected fault")
	interactive := flag.Bool("interactive", false, "request exact human approval from an attached terminal; default denies every risky action")
	pid := flag.Uint("pid", 0, "attach to one existing synthetic bank; requires one explicit tenant and oracle-state")
	oracleState := flag.String("oracle-state", "", "independent state file for the attached synthetic bank")
	timeout := flag.Duration("run-timeout", 60*time.Second, "per-run timeout, 1s..5m, followed by at most8s cleanup")
	flag.Parse()
	if *boundaries {
		return checkBoundaries()
	}
	tenantNames, err := validateOptions(*count, *tenants, *scenario, *pid, *oracleState, *timeout)
	if err != nil {
		return err
	}
	if flag.NArg() != 0 {
		return errors.New("unexpected argument")
	}
	if runtime.GOOS == "linux" && os.Getenv("XDG_SESSION_TYPE") != "x11" {
		return errors.New("qualification requires the logged-in X11 desktop session")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var human approver
	if *interactive && *scenario != "denial" {
		human, err = newTerminalApprover(ctx, os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
	}
	provenance, err := bindCampaign(ctx, *root, *broker, *bank, *verifier, *scenario)
	if err != nil {
		return err
	}
	a, err := app.New(ctx, app.Config{Root: *root, Broker: *broker, Verifier: *verifier, Bank: *bank})
	if err != nil {
		return err
	}
	defer a.Close()
	r := report{SchemaVersion: 3, Platform: runtime.GOOS, Architecture: runtime.GOARCH, DesktopSession: os.Getenv("XDG_SESSION_TYPE"), StartedAt: time.Now().UTC().Format(time.RFC3339), Mode: "fresh_process", Scenario: *scenario, RequestedPerTenant: *count, Tenants: tenantNames, Results: []result{}, Interactive: *interactive, Provenance: provenance}
	if *pid != 0 {
		r.Mode = "same_process"
	}
	if err = reserveReport(*out, r); err != nil {
		return err
	}
	defer func() { summarize(&r); returnErr = errors.Join(returnErr, save(*out, r)) }()
	haltReason := ""
	for _, tenant := range tenantNames {
		for index := 1; index <= *count; index++ {
			item := result{Tenant: tenant, Index: index, Scenario: *scenario, Verdict: "unavailable", Outcome: "blocked", Oracle: "not checked"}
			if ctx.Err() != nil {
				haltReason = ctx.Err().Error()
			}
			if haltReason == "" {
				if err = r.Provenance.check(); err != nil {
					haltReason = err.Error()
				}
			}
			if haltReason != "" {
				item.Error = "not attempted: " + haltReason
				r.Results = append(r.Results, item)
				continue
			}
			req := app.StartRequest{CapabilityPath: "examples/balance.json", ContractPath: "scenarios/balance-m1001.contract.json", Tenant: tenant, PID: uint32(*pid), Inputs: map[string]workflow.Value{"member_id": {Type: "string", Text: "M-1001"}}, Fault: "none"}
			if *scenario != "balance" {
				req.CapabilityPath = "examples/create-subaccount.json"
				req.ContractPath = "scenarios/subaccount.contract.json"
				req.Inputs["subaccount_name"] = workflow.Value{Type: "string", Text: "Qualification"}
				if *scenario != "denial" && *scenario != "creation" {
					req.Fault = *scenario
				}
			}
			start := time.Now()
			item.Attempted = true
			item.RunID, err = a.Start(req)
			var view app.RunView
			if err != nil {
				item.Error = err.Error()
			} else {
				runCtx, cancel := context.WithTimeout(ctx, *timeout)
				var waitErr error
				var approvals trialApprovals
				view, approvals, waitErr = waitTrialWithApproval(runCtx, a, item.RunID, human, expectedForm{tenant, "M-1001", "Qualification"})
				item.ApprovalDenied = approvals.Denied
				item.ApprovedActionIDs = approvals.Granted
				cancel()
				item.Phase = view.State.Phase
				item.EvidenceDir = view.EvidenceDir
				item.Error = view.Error
				if waitErr != nil {
					item.Error = errors.Join(waitErr, optionalError(item.Error)).Error()
				}
				if item.Error == "" && view.State.Phase != workflow.Completed {
					item.Error = view.State.Reason
				}
				if view.Report != nil {
					item.Verdict = view.Report.Verdict
				}
				if !view.Finished {
					haltReason = "prior native run did not finish bounded cancellation"
				}
				item.ObservationRetries, item.InputRetries = retryCounts(view.Records)
				item.FaultReached = faultReached(*scenario, view.Records, item.ApprovalDenied)
				statePath := *oracleState
				if statePath == "" {
					statePath = filepath.Join(a.Root(), ".local", "bank", item.RunID, "state.json")
				}
				expected := expectation(*scenario, len(approvals.Granted) > 0 && !approvals.Denied)
				oracleErr, taskOracleErr := scenarioOracles(statePath, tenant, *scenario, len(approvals.Granted) > 0 && !approvals.Denied)
				if oracleErr == nil {
					item.Oracle = expected.description()
					item.OracleVerified = true
				} else {
					item.Oracle = oracleErr.Error()
				}
				if taskOracleErr != nil {
					item.TaskOracleError = taskOracleErr.Error()
				}
				item.Outcome = classify(view, item.Verdict, item.Error, taskOracleErr)
				item.ExpectedBehaviorVerified = expectedBehavior(*scenario, view, &item, oracleErr, taskOracleErr)
			}
			item.LatencyMillis = time.Since(start).Milliseconds()
			r.Results = append(r.Results, item)
			summarize(&r)
			fmt.Fprintf(os.Stderr, "%s %d/%d %s phase=%s retries=%d %dms\n", tenant, index, *count, item.Outcome, item.Phase, item.ObservationRetries, item.LatencyMillis)
			if err = save(*out, r); err != nil {
				return err
			}
		}
	}
	summarize(&r)
	if err = json.NewEncoder(os.Stdout).Encode(r); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !r.CampaignPassed {
		return errors.New("campaign contains failed or blocked trials; see preserved report")
	}
	return nil
}
func optionalError(message string) error {
	if message == "" {
		return nil
	}
	return errors.New(message)
}
func validateOptions(count int, tenants, scenario string, pid uint, oracleState string, timeout time.Duration) ([]string, error) {
	if count < 1 || count > 100 || timeout < time.Second || timeout > 5*time.Minute {
		return nil, errors.New("n must be1..100 and run-timeout1s..5m")
	}
	names := strings.Split(tenants, ",")
	seen := map[string]bool{}
	for _, name := range names {
		if (name != "north" && name != "south") || seen[name] {
			return nil, errors.New("tenants must be unique north/south")
		}
		seen[name] = true
	}
	switch scenario {
	case "balance", "creation", "denial", "overlay", "delay", "missing-control", "duplicate-control", "commit-noop", "false-ack", "crash-after-commit":
	default:
		return nil, errors.New("unknown scenario")
	}
	if uint64(pid) > math.MaxUint32 {
		return nil, errors.New("pid exceeds platform identifier range")
	}
	if pid != 0 && (len(names) != 1 || oracleState == "" || scenario != "balance") {
		return nil, errors.New("pid requires exactly one explicit tenant, oracle-state, and the read-only balance scenario")
	}
	if pid == 0 && oracleState != "" {
		return nil, errors.New("oracle-state requires pid")
	}
	return names, nil
}

type trialHost interface {
	Get(string) (app.RunView, error)
	Control(context.Context, string, computer.Control) error
	Wait(context.Context, string) (app.RunView, error)
}

func cancelTrial(host trialHost, id string, view app.RunView, cause error) (app.RunView, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := host.Control(ctx, id, computer.Control{Kind: "cancel", Epoch: view.State.Epoch}); err != nil {
		return view, errors.Join(cause, err)
	}
	final, err := host.Wait(ctx, id)
	return final, errors.Join(cause, err)
}
func classify(view app.RunView, verdict, runError string, oracleErr error) string {
	var mismatch oracleMismatch
	if errors.As(oracleErr, &mismatch) || verdict == "failed" || view.State.Phase == workflow.Failed {
		return "failed"
	}
	if view.Finished && view.State.Phase == workflow.Completed && verdict == "passed" && runError == "" && oracleErr == nil {
		return "passed"
	}
	return "blocked"
}
func faultReached(scenario string, records []computer.Record, denied bool) bool {
	if scenario == "denial" {
		return denied
	}
	for _, record := range records {
		o := record.Observation
		if o == nil || !o.Complete {
			continue
		}
		names := map[string]int{}
		status := ""
		for _, node := range o.Nodes {
			names[node.Name]++
			if node.Role == "text_field" && node.Name == "Status" && node.Value != nil {
				status = *node.Value
			}
		}
		switch scenario {
		case "overlay":
			if names["Session notice"] > 0 && names["Dismiss"] > 0 {
				return true
			}
		case "delay":
			if status == "Searching" {
				return true
			}
		case "missing-control":
			if status == "Enter subaccount name" && names["Review"] == 0 {
				return true
			}
		case "duplicate-control":
			if names["Confirm creation"] > 1 {
				return true
			}
		case "commit-noop":
			if !denied && status == "Creation did not complete" {
				return true
			}
		}
	}
	// A launch flag, an approval denial or a process exit cannot prove that a
	// post-commit fault executed. Such campaigns remain explicitly unreached.
	return false
}
func summarize(r *report) {
	r.Passed = 0
	r.Failed = 0
	r.Blocked = 0
	r.FirstAttemptSuccesses = 0
	r.ExpectedBehaviorPasses = 0
	r.ByTenant = map[string]tenantMetrics{}
	seen := map[string]map[int]bool{}
	runIDs := map[string]bool{}
	valid := true
	for _, name := range r.Tenants {
		if (name != "north" && name != "south") || seen[name] != nil {
			valid = false
		}
		seen[name] = map[int]bool{}
		r.ByTenant[name] = tenantMetrics{Requested: r.RequestedPerTenant}
	}
	for _, item := range r.Results {
		if item.RunID != "" {
			if runIDs[item.RunID] {
				valid = false
			}
			runIDs[item.RunID] = true
		}
		indices, ok := seen[item.Tenant]
		if !ok || item.Index < 1 || item.Index > r.RequestedPerTenant || indices[item.Index] {
			valid = false
			continue
		}
		indices[item.Index] = true
		m := r.ByTenant[item.Tenant]
		if item.Attempted {
			m.Attempted++
		}
		if item.FaultReached {
			m.FaultReached++
		}
		if item.ExpectedBehaviorVerified && item.Attempted && item.OracleVerified && item.RunID != "" {
			r.ExpectedBehaviorPasses++
		}
		switch item.Outcome {
		case "passed":
			if !item.Attempted || !item.OracleVerified || item.RunID == "" || item.Phase != workflow.Completed || item.Verdict != "passed" || item.Error != "" {
				valid = false
			}
			r.Passed++
			m.Passed++
			if item.ObservationRetries == 0 && item.InputRetries == 0 {
				r.FirstAttemptSuccesses++
			}
		case "failed":
			r.Failed++
			m.Failed++
		case "blocked":
			r.Blocked++
			m.Blocked++
		default:
			valid = false
		}
		r.ByTenant[item.Tenant] = m
	}
	r.MatrixComplete = valid && len(r.Tenants) == 2 && r.Mode == "fresh_process"
	for _, name := range r.Tenants {
		r.MatrixComplete = r.MatrixComplete && len(seen[name]) == r.RequestedPerTenant && r.ByTenant[name].Attempted == r.RequestedPerTenant
	}
	r.Qualified = r.MatrixComplete && r.RequestedPerTenant >= 20 && len(r.Tenants) == 2 && r.Mode == "fresh_process" && r.Scenario == "balance" && r.Failed == 0 && r.Blocked == 0
	r.OneSided95LowerBound = lowerBound(r.Passed, len(r.Results))
	r.CampaignPassed = valid && len(r.Results) == len(r.Tenants)*r.RequestedPerTenant && r.ExpectedBehaviorPasses == len(r.Results)
}
func reserveReport(path string, r report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return errors.Join(json.NewEncoder(f).Encode(r), f.Close())
}
func save(path string, v report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".qualification-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func checkBoundaries() error {
	cmd := exec.Command("go", "list", "-deps", "github.com/bharathvbcr/Jarvis/internal/trace", "github.com/bharathvbcr/Manvi/manvi/workflow")
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(string(data)) {
		for _, forbidden := range []string{"/manvi/computer", "/manvi/llm", "net/http", "os/exec"} {
			if name == forbidden || strings.Contains(name, forbidden) {
				return fmt.Errorf("offline/pure workflow imports forbidden dependency %s", name)
			}
		}
	}
	fmt.Println("offline trace and workflow dependency boundaries passed")
	return nil
}

// Oracle is a qualification-only reader. It is neither registered as a tool nor
// called by discovery. Expected values are supplied independently of the UI.
type oracleMismatch struct{ message string }

func (e oracleMismatch) Error() string { return e.message }

func oracle(path, tenant string) error {
	return oracleWithExpectation(path, tenant, oracleExpectation{})
}
func lowerBound(successes, total int) float64 {
	if successes == 0 || total == 0 {
		return 0
	}
	if successes == total {
		return math.Pow(.05, 1/float64(total))
	}
	lo, hi := 0.0, 1.0
	for iteration := 0; iteration < 100; iteration++ {
		p := (lo + hi) / 2
		tail := 0.0
		for k := successes; k <= total; k++ {
			a, _ := math.Lgamma(float64(total + 1))
			b, _ := math.Lgamma(float64(k + 1))
			c, _ := math.Lgamma(float64(total - k + 1))
			tail += math.Exp(a - b - c + float64(k)*math.Log(p) + float64(total-k)*math.Log1p(-p))
		}
		if tail > .05 {
			hi = p
		} else {
			lo = p
		}
	}
	return (lo + hi) / 2
}
