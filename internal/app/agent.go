package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// OpClass says whether a one-shot agent call (`jarvis call`) may run a
// host-plane operation.
//
// The classes answer different questions, so they are checked in a fixed
// order: human-only first, then process-scoped. That order is what keeps an
// approval refused for its own reason when a transport that can reach a live
// run (a socket) is added later; were process scope checked first, lifting it
// would silently let an approval through.
type OpClass string

const (
	// OpAgent ops read, compile or diagnose from disk. A one-shot process can
	// answer them completely.
	OpAgent OpClass = "agent"
	// OpHumanOnly ops approve, act, take over, promote, set a credential, read
	// or write a sensitive input, or write an export. An agent must never
	// reach them: each is a human decision or a secret.
	OpHumanOnly OpClass = "human-only"
	// OpProcessScoped ops address a run or a background job held in the memory
	// of the process that started it. A fresh process has none, so a one-shot
	// call could only start something that dies with it.
	OpProcessScoped OpClass = "process-scoped"
	// OpRefused is an op this table does not classify: a Manvi host-plane
	// built-in, or a Jarvis op added without a classification.
	OpRefused OpClass = "refused"
)

// Error codes for an agent call the gate refused. They extend serve's codes:
// a host that does not know them treats them as generic failures.
const (
	ErrAgentRefused  = "E_AGENT_REFUSED"
	ErrProcessScoped = "E_PROCESS_SCOPED"
)

// AgentMode is how much an agent call may do. The default refuses every
// human-only op. Yolo is the owner's explicit choice to let agents operate
// Jarvis like a person; it is resolved only in cmd/jarvis, from --yolo or
// JARVIS_AGENT_MODE=yolo, and every run it starts is marked agent-operated.
type AgentMode string

const (
	AgentModeDefault AgentMode = ""
	AgentModeYolo    AgentMode = "yolo"
)

// ParseAgentMode reads a JARVIS_AGENT_MODE value. Anything unrecognised is
// an error rather than the default, so a typo cannot quietly change what an
// agent is allowed to do.
func ParseAgentMode(value string) (AgentMode, error) {
	switch strings.TrimSpace(value) {
	case "", "default":
		return AgentModeDefault, nil
	case "yolo":
		return AgentModeYolo, nil
	}
	return "", fmt.Errorf("agent mode %q is neither default nor yolo", value)
}

type opPolicy struct {
	class OpClass
	// yolo admits a human-only op to a one-shot call in yolo mode: it acts on
	// disk, so a fresh process can complete it.
	yolo bool
	// live marks a human-only op that needs a run or credential held by a
	// live process. Yolo reaches those through --events, not call.
	live bool
	// async ops answer with a job_id that jarvis.job.get must be polled for.
	async bool
	// inline allows params on the command line. Off for ops whose params can
	// carry input values, so those stay out of argv and shell history.
	inline bool
	// paths names the params that are filesystem paths. Each must resolve
	// inside the product root and outside its private .local state.
	paths []string
}

// agentOps classifies every jarvis.* operation App.Configure registers.
// TestEveryServedJarvisOpIsClassified fails when an op is added without a
// row here, so the gate is default-deny rather than default-allow.
var agentOps = map[string]opPolicy{
	"jarvis.catalog.list":       {class: OpAgent, inline: true},
	"jarvis.catalog.history":    {class: OpAgent, inline: true},
	"jarvis.capability.read":    {class: OpAgent, inline: true, paths: []string{"path"}},
	"jarvis.capability.compile": {class: OpAgent, inline: true},
	"jarvis.codegen":            {class: OpAgent, inline: true, paths: []string{"capability_path"}},
	"jarvis.trace.load":         {class: OpAgent, inline: true, paths: []string{"path"}},
	"jarvis.trace.replay":       {class: OpAgent, paths: []string{"path", "capability_path"}},
	"jarvis.inputs.list-keys":   {class: OpAgent, inline: true},
	"jarvis.doctor":             {class: OpAgent, async: true, inline: true},
	"jarvis.catalog.promote":    {class: OpHumanOnly, yolo: true, inline: true},
	"jarvis.inputs.put":         {class: OpHumanOnly, yolo: true},
	"jarvis.run.assist":         {class: OpHumanOnly, live: true},
	"jarvis.run.takeover":       {class: OpHumanOnly, live: true},
	"jarvis.run.handback":       {class: OpHumanOnly, live: true},
	"jarvis.run.act":            {class: OpHumanOnly, live: true},
	"jarvis.key.set":            {class: OpHumanOnly, live: true},
	"jarvis.evidence.export":    {class: OpHumanOnly, live: true},
	// inputs.get prints a stored secret into the caller's transcript. That is
	// disclosure, not a skipped approval, and no run needs it: runs read
	// inputs through --inputs. It stays refused in every mode.
	"jarvis.inputs.get":        {class: OpHumanOnly},
	"jarvis.run.start":         {class: OpProcessScoped},
	"jarvis.run.list":          {class: OpProcessScoped},
	"jarvis.run.get":           {class: OpProcessScoped},
	"jarvis.run.control":       {class: OpProcessScoped},
	"jarvis.intervention.list": {class: OpProcessScoped},
	"jarvis.anchor.create":     {class: OpProcessScoped, async: true},
	"jarvis.frame.get":         {class: OpProcessScoped, async: true},
	"jarvis.evidence.verify":   {class: OpProcessScoped, async: true},
	"jarvis.job.get":           {class: OpProcessScoped},
	"jarvis.discovery.start":   {class: OpProcessScoped},
	"jarvis.discovery.get":     {class: OpProcessScoped},
}

// humanControls are the jarvis.run.control kinds only a person may send.
// The rest (deny, cancel, pause, refresh, focus) can only stop or re-observe.
var humanControls = []string{"approve", "resume"}

// OpInfo describes one served op for `jarvis ops`.
type OpInfo struct {
	Op           string   `json:"op"`
	Class        OpClass  `json:"class"`
	Async        bool     `json:"async,omitempty"`
	InlineParams bool     `json:"inline_params,omitempty"`
	PathParams   []string `json:"path_params,omitempty"`
	// Yolo reports that `jarvis call --yolo` admits this human-only op.
	Yolo bool   `json:"yolo,omitempty"`
	Note string `json:"note,omitempty"`
}

// AgentOps classifies the ops a server reported in hello, sorted by name.
func AgentOps(served []string) []OpInfo {
	out := make([]OpInfo, 0, len(served))
	for _, op := range served {
		info := OpInfo{Op: op, Class: OpRefused}
		switch p, ok := agentOps[op]; {
		case op == serve.OpHello:
			info.Class = OpAgent
		case ok:
			info.Class, info.Async, info.InlineParams, info.Yolo = p.class, p.async, p.inline, p.yolo
			info.PathParams = append([]string(nil), p.paths...)
			switch {
			case op == "jarvis.run.control":
				info.Note = "control kinds " + strings.Join(humanControls, " and ") + " are human-only; in yolo mode answer them on replay/discover --events --yolo stdin"
			case p.live:
				info.Note = "needs a live run or credential; in yolo mode use replay/discover --events --yolo"
			}
		case strings.HasPrefix(op, "jarvis."):
			info.Note = "not classified for agents"
		default:
			info.Note = "Manvi host-plane built-in; use the manvi CLI"
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Op < out[j].Op })
	return out
}

// AgentAsync reports whether op answers with a background job to poll.
func AgentAsync(op string) bool { return agentOps[op].async }

// AdmitAgent decides whether a one-shot agent call may send op with params.
// inline reports that params came from the command line. Nil admits.
func (a *App) AdmitAgent(op string, params json.RawMessage, inline bool, mode AgentMode) *serve.Error {
	if op == serve.OpHello {
		return nil
	}
	p, ok := agentOps[op]
	if !ok {
		if strings.HasPrefix(op, "jarvis.") {
			return refuse(ErrAgentRefused, "%s is not classified for agent calls, so it is refused", op)
		}
		return refuse(ErrAgentRefused, "%s is a Manvi host-plane built-in; use the manvi CLI", op)
	}
	if op == "jarvis.run.control" && mode != AgentModeYolo {
		if kind, needs := controlNeedsHuman(params); needs {
			return refuse(ErrAgentRefused, "jarvis.run.control kind %q is a human decision; approve it at a terminal (jarvis replay) or in the workbench", kind)
		}
	}
	switch {
	case p.class == OpHumanOnly && mode == AgentModeYolo && p.live:
		return refuse(ErrProcessScoped, "%s needs a live run or credential that a one-shot call does not hold; in yolo mode answer it on jarvis replay|discover --events --yolo stdin, and set GEMINI_API_KEY in the environment", op)
	case p.class == OpHumanOnly && mode == AgentModeYolo && p.yolo:
	case p.class == OpHumanOnly && mode == AgentModeYolo:
		return refuse(ErrAgentRefused, "%s stays refused in yolo mode: it prints a stored secret and no run needs it", op)
	case p.class == OpHumanOnly:
		return refuse(ErrAgentRefused, "%s is human-only: it approves, acts, promotes, sets a credential, reads or writes a sensitive input, or writes an export", op)
	case p.class == OpProcessScoped:
		return refuse(ErrProcessScoped, "%s addresses a run or job held in the memory of the process that started it; a one-shot call has none. Use jarvis replay --events or jarvis discover --events", op)
	case p.class == OpAgent:
	default:
		return refuse(ErrAgentRefused, "%s has no usable classification", op)
	}
	if inline && !p.inline {
		return refuse(ErrAgentRefused, "%s takes params only from --params @FILE or --params - (stdin), never the command line", op)
	}
	if len(p.paths) == 0 {
		return nil
	}
	fields, err := objectFields(params)
	if err != nil {
		return refuse(serve.ErrBadRequest, "%s params: %v", op, err)
	}
	for key, value := range fields {
		for _, name := range p.paths {
			// encoding/json matches field names case-insensitively, so "PATH"
			// reaches the handler's Path field and must be checked as one.
			if !strings.EqualFold(key, name) {
				continue
			}
			var path string
			if err := json.Unmarshal(value, &path); err != nil {
				return refuse(serve.ErrBadRequest, "%s param %q must be a string", op, key)
			}
			if err := a.agentPath(path); err != nil {
				return refuse(ErrAgentRefused, "%s param %q: %v", op, key, err)
			}
		}
	}
	return nil
}

// agentPath admits a path an agent op will read: it must resolve, through
// any symlinks, to an existing file inside the product root and outside
// .local, which holds sensitive inputs, provider-private discovery sessions
// and promotion records.
func (a *App) agentPath(path string) error {
	root, err := filepath.EvalSymlinks(a.cfg.Root)
	if err != nil {
		return fmt.Errorf("product root is not readable: %w", err)
	}
	target, err := filepath.EvalSymlinks(a.resolve(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("path does not exist")
		}
		return fmt.Errorf("path is not readable: %w", err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("path resolves outside the product root")
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	// EqualFold: the default macOS filesystem is case-insensitive, so
	// ".LOCAL" is the same directory.
	if strings.EqualFold(first, ".local") {
		return errors.New("path resolves into .local, which holds private state")
	}
	return nil
}

// controlNeedsHuman reports whether jarvis.run.control params carry a kind
// only a person may send. Params it cannot read are treated as needing one.
func controlNeedsHuman(params json.RawMessage) (string, bool) {
	fields, err := objectFields(params)
	if err != nil {
		return "unreadable", true
	}
	for key, value := range fields {
		if !strings.EqualFold(key, "control") {
			continue
		}
		control, err := objectFields(value)
		if err != nil {
			return "unreadable", true
		}
		for k, v := range control {
			if !strings.EqualFold(k, "kind") {
				continue
			}
			var kind string
			if err := json.Unmarshal(v, &kind); err != nil {
				return "unreadable", true
			}
			for _, human := range humanControls {
				if strings.EqualFold(strings.TrimSpace(kind), human) {
					return kind, true
				}
			}
		}
	}
	return "", false
}

// objectFields decodes a params object with the handlers' own strict
// decoder, so a document the gate reads is the one the handler would.
func objectFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := workflow.DecodeStrict(raw, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("params must be a JSON object")
	}
	return fields, nil
}

func refuse(code, format string, args ...any) *serve.Error {
	return &serve.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
