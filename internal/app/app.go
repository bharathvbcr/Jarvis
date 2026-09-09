package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/credentials"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
	"github.com/bharathvbcr/Manvi/manvi/workflow/catalog"
)

type Config struct{ Root, Broker, Verifier, Bank string }
type App struct {
	jobs        jobRegistry
	cfg         Config
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	active      string
	runs        map[string]*runEntry
	client      *computer.Client
	credentials *credentials.Resolver
}
type StartRequest struct {
	CapabilityPath string                    `json:"capability_path"`
	ContractPath   string                    `json:"contract_path"`
	Tenant         string                    `json:"tenant"`
	Inputs         map[string]workflow.Value `json:"inputs"`
	PID            uint32                    `json:"pid,omitempty"`
	Fault          string                    `json:"fault,omitempty"`
	Assisted       bool                      `json:"assisted,omitempty"`
}
type RunView struct {
	Admission         computer.AttachmentReadiness `json:"admission"`
	RunID             string                       `json:"run_id"`
	State             workflow.State               `json:"state"`
	PendingActionID   string                       `json:"pending_action_id"`
	ProgramGeneration uint64                       `json:"program_generation"`
	Capability        workflow.Capability          `json:"capability"`
	EvidenceDir       string                       `json:"evidence_dir"`
	Finished          bool                         `json:"finished"`
	Error             string                       `json:"error,omitempty"`
	Report            *devcouncil.EvidenceReport   `json:"report,omitempty"`
	Observation       *computer.Observation        `json:"observation,omitempty"`
	Records           []computer.Record            `json:"records"`
	TotalRecords      int                          `json:"total_records"`
	Assistance        string                       `json:"assistance,omitempty"`
	Assisted          bool                         `json:"assisted"`
	Reconciliation    *Reconciliation              `json:"reconciliation,omitempty"`
	Matching          *computer.MatchDiagnostics   `json:"matching,omitempty"`
}
type runEntry struct {
	mu                sync.Mutex
	startedAt         time.Time
	program           *workflow.Program
	privacy           computer.PrivacyPolicy
	view              RunView
	run               *computer.Run
	cancel            context.CancelFunc
	session           computer.Session
	expected          devcouncil.EvidenceRequest
	done              chan struct{}
	assisted          bool
	assistanceRunning bool
	assistanceUsed    bool
}

func New(ctx context.Context, cfg Config) (*App, error) {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	cfg.Root = root
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if cfg.Broker == "" {
		cfg.Broker = filepath.Join(root, "build", "manvi-desktop"+suffix)
	}
	if cfg.Verifier == "" {
		cfg.Verifier = filepath.Join(root, "build", "dcverify"+suffix)
	}
	if cfg.Bank == "" {
		cfg.Bank = filepath.Join(root, "build", "jarvis-bank"+suffix)
	}
	ctx, cancel := context.WithCancel(ctx)
	return &App{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[string]*runEntry{}, credentials: credentials.NewResolver()}, nil
}
func (a *App) Close() error {
	a.cancel()
	a.mu.Lock()
	c := a.client
	e := a.runs[a.active]
	a.mu.Unlock()
	if e != nil {
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-e.done:
			timer.Stop()
		case <-timer.C:
		}
	}
	if c != nil {
		return c.Close()
	}
	return nil
}
func (a *App) Root() string { return a.cfg.Root }
func (a *App) Catalog() catalog.Store {
	return catalog.Store{Root: filepath.Join(a.cfg.Root, ".local", "catalog")}
}
func (a *App) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(a.cfg.Root, path)
}
func (a *App) Compile(path string) (*workflow.Program, error) {
	raw, err := readBounded(a.resolve(path), workflow.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	return workflow.Compile(raw)
}
func (a *App) broker() (*computer.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	if a.client != nil {
		return a.client, nil
	}
	c, err := computer.Start(a.ctx, a.cfg.Broker)
	if err != nil {
		return nil, err
	}
	a.client = c
	return c, nil
}
func identifier() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func (a *App) Start(req StartRequest) (string, error) {
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	if req.Tenant != "north" && req.Tenant != "south" {
		return "", errors.New("tenant must be north or south")
	}
	switch req.Fault {
	case "", "none", "overlay", "delay", "missing-control", "duplicate-control", "commit-noop", "false-ack", "crash-after-commit":
	default:
		return "", errors.New("unsupported bank fault")
	}
	p, err := a.Compile(req.CapabilityPath)
	if err != nil {
		return "", err
	}
	if p.Capability().Application != "jarvis-bank" {
		return "", errors.New("this product profile only admits jarvis-bank capabilities")
	}
	if err = p.ValidateInputs(req.Inputs); err != nil {
		return "", err
	}
	profile, _, err := a.profile(req.Tenant)
	if err != nil {
		return "", err
	}
	if err = validateBankProgram(p, profile); err != nil {
		return "", err
	}
	contract, err := readBounded(a.resolve(req.ContractPath), workflow.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	var contractDoc devcouncil.EvidenceContract
	if err = workflow.DecodeStrict(contract, &contractDoc); err != nil {
		return "", err
	}
	if contractDoc.SchemaVersion != 1 || len(contractDoc.Criteria) == 0 {
		return "", errors.New("independent acceptance contract required")
	}
	id, err := identifier()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	entry := &runEntry{startedAt: time.Now().UTC(), program: p, cancel: cancel, done: make(chan struct{}), view: RunView{RunID: id, State: workflow.State{Phase: "admitted"}, Capability: p.Capability(), EvidenceDir: filepath.Join(a.cfg.Root, "evidence", "private", id), Records: []computer.Record{}}}
	entry.assisted = req.Assisted
	entry.view.ProgramGeneration = 1
	inputs := make(map[string]workflow.Value, len(req.Inputs))
	for k, v := range req.Inputs {
		inputs[k] = v
	}
	req.Inputs = inputs
	a.mu.Lock()
	if a.active != "" {
		a.mu.Unlock()
		cancel()
		return "", errors.New("interactive desktop already owned by run " + a.active)
	}
	if len(a.runs) >= 512 {
		a.mu.Unlock()
		cancel()
		return "", errors.New("512-run session limit reached; restart host after exporting evidence")
	}
	a.active = id
	a.runs[id] = entry
	a.mu.Unlock()
	go a.execute(ctx, entry, p, contract, req)
	return id, nil
}
func (a *App) execute(ctx context.Context, e *runEntry, p *workflow.Program, contract []byte, req StartRequest) {
	var failure error
	defer func() {
		e.cancel()
		a.mu.Lock()
		if a.active == e.view.RunID {
			a.active = ""
		}
		a.mu.Unlock()
		e.mu.Lock()
		if failure != nil {
			e.view.Error = failure.Error()
		}
		e.view.Finished = true
		e.mu.Unlock()
		close(e.done)
	}()
	client, err := a.broker()
	if err != nil {
		failure = err
		return
	}
	profile, bindingBytes, err := a.profile(req.Tenant)
	if err != nil {
		failure = err
		return
	}
	pid := req.PID
	var bank *ownedBank
	if pid == 0 {
		bankDir := filepath.Join(a.cfg.Root, ".local", "bank", e.view.RunID)
		if err = os.MkdirAll(bankDir, 0700); err != nil {
			failure = err
			return
		}
		args := []string{"--tenant", req.Tenant, "--state", filepath.Join(bankDir, "state.json")}
		if req.Fault != "" {
			args = append(args, "--fault", req.Fault)
		}
		bank, err = startOwnedBank(ctx, a.cfg.Bank, args...)
		if err != nil {
			failure = err
			return
		}
		pid = uint32(bank.cmd.Process.Pid)
		defer func() {
			failure = errors.Join(failure, bank.stop())
		}()

	}
	attachCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	session, admission, err := client.AttachFocusedReady(attachCtx, e.view.RunID, pid, profile.ReadOnlyTargets)
	cancel()
	e.mu.Lock()
	e.view.Admission = admission
	e.mu.Unlock()
	if err != nil {
		err = bank.admissionError(err)
		failure = errors.Join(err, a.discardBroker(client))
		return
	}
	defer func() {
		e.mu.Lock()
		s := e.session
		if e.run != nil {
			s.Epoch = e.run.Snapshot().Epoch
		}
		e.mu.Unlock()
		if s.ID == "" {
			s = session
		}
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := client.Control(c, s, "detach"); err != nil && failure == nil {
			failure = err
		}
	}()
	if session.Window.Title != profile.WindowTitle {
		failure = errors.New("attached window does not match the bank profile")
		return
	}
	recorder, err := NewRecorder(e.view.EvidenceDir, p, contract, session)
	if err != nil {
		failure = err
		return
	}
	if err = writeAtomic(filepath.Join(e.view.EvidenceDir, "binding.json"), bindingBytes); err != nil {
		recorder.file.Close()
		failure = err
		return
	}
	e.mu.Lock()
	e.session = session
	e.expected = devcouncil.EvidenceRequest{Binary: a.cfg.Verifier, ContractPath: filepath.Join(e.view.EvidenceDir, "contract.json"), BundlePath: filepath.Join(e.view.EvidenceDir, "bundle.json"), ArtifactsRoot: e.view.EvidenceDir, ContractSHA256: digest(contract), CapabilitySHA256: p.Digest(), RunID: session.RunID, SessionID: session.ID, Epoch: session.Epoch}
	e.mu.Unlock()
	onRecord := func(record computer.Record) error {
		if err := recorder.Record(record); err != nil {
			return err
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		var owned computer.Record
		if err = json.Unmarshal(raw, &owned); err != nil {
			return err
		}
		if owned.Observation != nil {
			owned.Observation.Screenshot.Base64 = ""
		}
		e.mu.Lock()
		e.view.Records = append(e.view.Records, owned)
		e.mu.Unlock()
		return nil
	}
	privacy := bankPrivacy(req.Inputs)
	privacy.PID = session.Window.PID
	privacy.WindowID = session.Window.ID
	e.mu.Lock()
	e.privacy = privacy
	e.mu.Unlock()
	run, err := computer.StartRun(ctx, computer.RunOptions{Program: p, Inputs: req.Inputs, Desktop: client, Session: session, Privacy: privacy, OnRecord: onRecord, Assisted: req.Assisted})
	if err != nil {
		failure = err
		recorder.file.Close()
		return
	}
	e.mu.Lock()
	e.run = run
	e.mu.Unlock()
	result, runErr := run.Wait(context.Background())
	if result.State.Phase == workflow.Unknown {
		fenced := session
		fenced.Epoch = result.State.Epoch
		reconciliation, observation := observeUnknown(ctx, client, fenced, privacy, result.State, recorder.lastActionID())
		if observation != nil {
			if err = onRecord(computer.Record{Kind: "observation", Stage: "outcome_reconciliation", ActionID: reconciliation.ActionID, Actor: "coordinator", Epoch: fenced.Epoch, Observation: observation}); err != nil {
				failure = err
				recorder.file.Close()
				return
			}
		}
		if err = recorder.RecordReconciliation(reconciliation); err != nil {
			failure = err
			recorder.file.Close()
			return
		}
		e.mu.Lock()
		e.view.Reconciliation = &reconciliation
		e.mu.Unlock()
	}
	if err = recorder.Finish(result); err != nil {
		failure = err
		return
	}
	e.mu.Lock()
	e.view.State = result.State
	e.view.Observation = run.Observation()
	expected := e.expected
	e.mu.Unlock()
	report, err := devcouncil.VerifyEvidence(ctx, expected)
	if err != nil {
		failure = err
	} else {
		if err = writeJSON(filepath.Join(e.view.EvidenceDir, "verification.json"), report); err != nil {
			failure = err
		}
		e.mu.Lock()
		e.view.Report = &report
		e.mu.Unlock()
	}
	if runErr != nil && failure == nil {
		failure = runErr
	}
}
func bankReadTargets() []computer.Selector {
	out := []computer.Selector{}
	for _, name := range []string{"Search", "New subaccount", "Review", "Back", "Dismiss"} {
		out = append(out, computer.Selector{Role: "button", Name: name})
	}
	for _, name := range []string{"Member ID", "Subaccount name"} {
		out = append(out, computer.Selector{Role: "text_field", Name: name})
	}
	return out
}

// An interrupted admission may have created a session whose identity never
// reached the host. Retire the broker and every helper before another owner is
// admitted; a lost attach response cannot leave a hidden desktop session.
func (a *App) discardBroker(c *computer.Client) error {
	err := c.Close()
	a.mu.Lock()
	if a.client == c {
		a.client = nil
	}
	a.mu.Unlock()
	return err
}
func (a *App) entry(id string) (*runEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	e := a.runs[id]
	if e == nil {
		return nil, errors.New("unknown run")
	}
	return e, nil
}
func (a *App) Get(id string) (RunView, error) {
	e, err := a.entry(id)
	if err != nil {
		return RunView{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	v := e.view
	v.Assisted = e.assisted
	v.TotalRecords = len(v.Records)
	if e.run != nil {
		v.State = e.run.Snapshot()
		v.Observation = e.run.Observation()
	}
	if v.State.StepIndex >= 0 && v.State.StepIndex < len(v.Capability.Steps) {
		if e.program != nil {
			v.PendingActionID = v.State.ActionID(e.program)
		}
		if v.Observation != nil {
			diagnostic := computer.Diagnose(v.Capability.Targets[v.Capability.Steps[v.State.StepIndex].Target], *v.Observation)
			v.Matching = &diagnostic
		}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return RunView{}, err
	}
	var owned RunView
	err = json.Unmarshal(raw, &owned)
	return owned, err
}

// RunList deliberately excludes inputs, outputs, frames and tool records. Full
// detail is fetched only for the selected identity through Get.
type RunList struct {
	Runs         []RunSummary `json:"runs"`
	TotalRuns    int          `json:"total_runs"`
	Truncated    bool         `json:"truncated"`
	DesktopOwner string       `json:"desktop_owner,omitempty"`
}
type RunSummary struct {
	RunID       string          `json:"run_id"`
	StartedAt   time.Time       `json:"started_at"`
	State       RunStateSummary `json:"state"`
	Finished    bool            `json:"finished"`
	Error       string          `json:"error,omitempty"`
	EvidenceDir string          `json:"evidence_dir"`
}
type RunStateSummary struct {
	SessionID string         `json:"session_id"`
	Epoch     uint64         `json:"epoch"`
	Phase     workflow.Phase `json:"phase"`
}

func (a *App) ListRuns() RunList {
	a.mu.Lock()
	entries := make([]*runEntry, 0, len(a.runs))
	for _, e := range a.runs {
		entries = append(entries, e)
	}
	owner := a.active
	a.mu.Unlock()
	out := RunList{Runs: make([]RunSummary, 0, len(entries)), TotalRuns: len(entries), DesktopOwner: owner}
	for _, e := range entries {
		e.mu.Lock()
		s := e.view.State
		if e.run != nil {
			s = e.run.Snapshot()
		}
		out.Runs = append(out.Runs, RunSummary{RunID: e.view.RunID, StartedAt: e.startedAt, State: RunStateSummary{SessionID: s.SessionID, Epoch: s.Epoch, Phase: s.Phase}, Finished: e.view.Finished, Error: e.view.Error, EvidenceDir: e.view.EvidenceDir})
		e.mu.Unlock()
	}
	sort.Slice(out.Runs, func(i, j int) bool {
		if out.Runs[i].StartedAt.Equal(out.Runs[j].StartedAt) {
			return out.Runs[i].RunID < out.Runs[j].RunID
		}
		return out.Runs[i].StartedAt.After(out.Runs[j].StartedAt)
	})
	return out
}
func (a *App) Control(ctx context.Context, id string, c computer.Control) error {
	e, err := a.entry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	run := e.run
	done := e.view.Finished
	e.mu.Unlock()
	if done {
		return errors.New("run is retired")
	}
	if c.Kind == "cancel" {
		if run != nil && c.Epoch != run.Snapshot().Epoch {
			return errors.New("stale control epoch")
		}
		e.cancel()
		return nil
	}
	if run == nil {
		return errors.New("run has not attached yet")
	}
	return run.Send(ctx, c)
}
func (a *App) Wait(ctx context.Context, id string) (RunView, error) {
	e, err := a.entry(id)
	if err != nil {
		return RunView{}, err
	}
	select {
	case <-ctx.Done():
		return RunView{}, ctx.Err()
	case <-e.done:
		return a.Get(id)
	}
}

type Diagnostic struct {
	Platform         string          `json:"platform"`
	Architecture     string          `json:"architecture"`
	Broker           json.RawMessage `json:"broker,omitempty"`
	Error            string          `json:"error,omitempty"`
	GeminiConfigured bool            `json:"gemini_configured"`
	Binaries         map[string]bool `json:"binaries"`
	Qualification    string          `json:"qualification"`
}

func (a *App) Doctor(ctx context.Context) Diagnostic {
	d := Diagnostic{Platform: runtime.GOOS, Architecture: runtime.GOARCH, Binaries: map[string]bool{}, Qualification: "not_tested"}
	for name, path := range map[string]string{"broker": a.cfg.Broker, "bank": a.cfg.Bank, "verifier": a.cfg.Verifier} {
		info, err := os.Stat(path)
		d.Binaries[name] = err == nil && info.Mode().IsRegular()
	}
	_, err := a.credentials.Resolve("gemini")
	d.GeminiConfigured = err == nil
	c, err := a.broker()
	if err != nil {
		d.Error = err.Error()
		return d
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	d.Broker, err = c.Probe(ctx)
	if err != nil {
		d.Error = err.Error()
	}
	return d
}
