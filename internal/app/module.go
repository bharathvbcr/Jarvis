package app

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	offline "github.com/bharathvbcr/Jarvis/internal/trace"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/credentials"
	"github.com/bharathvbcr/Manvi/manvi/devcouncil"
	"github.com/bharathvbcr/Manvi/manvi/serve"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
	"github.com/bharathvbcr/Manvi/manvi/workflow/catalog"
)

func handler[P, R interface{}](fn func(context.Context, P) (R, error)) serve.Handler {
	return func(ctx context.Context, raw json.RawMessage) (interface{}, *serve.Error) {
		var p P
		if len(raw) == 0 {
			raw = json.RawMessage(`{}`)
		}
		if err := workflow.DecodeStrict(raw, &p); err != nil {
			return nil, &serve.Error{Code: serve.ErrBadRequest, Message: err.Error()}
		}
		v, err := fn(ctx, p)
		if err != nil {
			return nil, &serve.Error{Code: "E_JARVIS", Message: err.Error()}
		}
		return v, nil
	}
}

type empty struct{}
type runID struct {
	RunID string `json:"run_id"`
}
type pathRequest struct {
	Path string `json:"path"`
}
type compileRequest struct {
	Source   string `json:"source"`
	Register bool   `json:"register"`
}
type compileResponse struct {
	Digest     string              `json:"digest"`
	Capability workflow.Capability `json:"capability"`
	Source     string              `json:"source,omitempty"`
	Path       string              `json:"path,omitempty"`
}
type promoteRequest struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}
type promotion struct {
	ID         string `json:"id"`
	Revision   string `json:"revision"`
	SHA256     string `json:"sha256"`
	PromotedAt string `json:"promoted_at"`
}

func (a *App) Configure(r *serve.Router) error {
	ops := map[string]serve.Handler{
		"jarvis.catalog.list": handler(func(_ context.Context, _ empty) (struct {
			Entries []catalog.Entry `json:"entries"`
		}, error) {
			v, err := a.Catalog().List()
			return struct {
				Entries []catalog.Entry `json:"entries"`
			}{v}, err
		}),
		"jarvis.capability.read": handler(func(_ context.Context, p pathRequest) (compileResponse, error) {
			v, err := a.Compile(p.Path)
			if err != nil {
				return compileResponse{}, err
			}
			return compileResponse{Digest: v.Digest(), Capability: v.Capability(), Source: string(v.Bytes()), Path: a.resolve(p.Path)}, nil
		}),
		"jarvis.capability.compile": handler(func(_ context.Context, p compileRequest) (compileResponse, error) {
			v, err := workflow.Compile([]byte(p.Source))
			if err != nil {
				return compileResponse{}, err
			}
			out := compileResponse{Digest: v.Digest(), Capability: v.Capability()}
			if p.Register {
				e, err := a.Catalog().Put(v)
				if err != nil {
					return out, err
				}
				out.Path = e.Path
			}
			return out, nil
		}),
		"jarvis.catalog.promote": handler(func(_ context.Context, p promoteRequest) (empty, error) {
			entries, err := a.Catalog().List()
			if err != nil {
				return empty{}, err
			}
			for _, e := range entries {
				if e.ID == p.ID && e.Revision == p.Revision {
					if err = a.Catalog().Promote(e.ID, e.Revision, e.SHA256); err != nil {
						return empty{}, err
					}
					history, err := a.history(e.ID)
					if err != nil {
						return empty{}, err
					}
					history = append(history, promotion{e.ID, e.Revision, e.SHA256, time.Now().UTC().Format(time.RFC3339Nano)})
					return empty{}, writeJSON(filepath.Join(a.cfg.Root, ".local", "promotions", e.ID+".json"), history)
				}
			}
			return empty{}, errors.New("unknown revision")
		}),
		"jarvis.catalog.history": handler(func(_ context.Context, p struct {
			ID string `json:"id"`
		}) (struct {
			History []promotion `json:"history"`
		}, error) {
			h, err := a.history(p.ID)
			return struct {
				History []promotion `json:"history"`
			}{h}, err
		}),
		"jarvis.run.start":     handler(func(_ context.Context, p StartRequest) (runID, error) { id, err := a.Start(p); return runID{id}, err }),
		"jarvis.run.list":      handler(func(_ context.Context, _ empty) (RunList, error) { return a.ListRuns(), nil }),
		"jarvis.anchor.create": asyncHandler(a, "jarvis.anchor.create", 15*time.Second, a.CreateAnchor),
		"jarvis.run.assist": handler(func(_ context.Context, p runID) (struct {
			Admitted bool `json:"admitted"`
		}, error) {
			err := a.Assist(p.RunID)
			return struct {
				Admitted bool `json:"admitted"`
			}{err == nil}, err
		}),
		"jarvis.run.get": handler(func(_ context.Context, p struct {
			RunID         string `json:"run_id"`
			CursorRecords int    `json:"cursor_records,omitempty"`
		}) (RunView, error) {
			v, err := a.Get(p.RunID)
			if err != nil {
				return v, err
			}
			if p.CursorRecords < 0 || p.CursorRecords > len(v.Records) {
				return v, errors.New("record cursor out of range")
			}
			v.Records = v.Records[p.CursorRecords:]
			return v, nil
		}),
		"jarvis.run.control": handler(func(ctx context.Context, p struct {
			RunID   string           `json:"run_id"`
			Control computer.Control `json:"control"`
		}) (empty, error) {
			return empty{}, a.Control(ctx, p.RunID, p.Control)
		}),
		"jarvis.intervention.list": handler(func(_ context.Context, p runID) (struct {
			Interventions []computer.InterventionRequest `json:"interventions"`
		}, error) {
			list, err := a.ListInterventions(p.RunID)
			return struct {
				Interventions []computer.InterventionRequest `json:"interventions"`
			}{list}, err
		}),
		"jarvis.run.takeover": handler(func(ctx context.Context, p runID) (empty, error) {
			return empty{}, a.Takeover(ctx, p.RunID)
		}),
		"jarvis.run.handback": handler(func(ctx context.Context, p runID) (empty, error) {
			return empty{}, a.Handback(ctx, p.RunID)
		}),
		"jarvis.run.act": handler(func(ctx context.Context, p struct {
			RunID string `json:"run_id"`
			Spec  string `json:"spec"`
		}) (empty, error) {
			return empty{}, a.ActHuman(ctx, p.RunID, p.Spec)
		}),
		"jarvis.doctor": asyncHandler(a, "jarvis.doctor", 10*time.Second, func(ctx context.Context, _ empty) (Diagnostic, error) {
			d := a.Doctor(ctx)
			if err := ctx.Err(); err != nil {
				return d, err
			}
			return d, nil
		}),
		"jarvis.job.get": handler(func(_ context.Context, p struct {
			ID string `json:"id"`
		}) (JobView, error) {
			return a.jobs.get(p.ID)
		}),
		"jarvis.key.set": handler(func(_ context.Context, p struct {
			Key string `json:"key"`
		}) (struct {
			Configured bool `json:"configured"`
		}, error) {
			if len(p.Key) < 16 || len(p.Key) > 4096 {
				return struct {
					Configured bool `json:"configured"`
				}{}, errors.New("credential length is invalid")
			}
			a.credentials.Set("gemini", credentials.NewSecret(p.Key, "workbench-memory"))
			return struct {
				Configured bool `json:"configured"`
			}{true}, nil
		}),
		"jarvis.evidence.verify": asyncHandler(a, "jarvis.evidence.verify", 15*time.Second, func(ctx context.Context, p runID) (devcouncil.EvidenceReport, error) {
			e, err := a.entry(p.RunID)
			if err != nil {
				return devcouncil.EvidenceReport{}, err
			}
			e.mu.Lock()
			expected, finished := e.expected, e.view.Finished
			e.mu.Unlock()
			if !finished {
				return devcouncil.EvidenceReport{}, errors.New("run is still active")
			}
			return devcouncil.VerifyEvidence(ctx, expected)
		}),
		"jarvis.codegen": handler(func(_ context.Context, p struct {
			CapabilityPath string `json:"capability_path"`
			PackageName    string `json:"package_name"`
		}) (struct {
			Source string `json:"source"`
		}, error) {
			v, err := a.Compile(p.CapabilityPath)
			if err != nil {
				return struct {
					Source string `json:"source"`
				}{}, err
			}
			if p.PackageName != "" && p.PackageName != "capability" {
				return struct {
					Source string `json:"source"`
				}{}, errors.New("generated package name is capability")
			}
			b, err := catalog.GenerateGo(v)
			return struct {
				Source string `json:"source"`
			}{string(b)}, err
		}),
		"jarvis.frame.get": asyncHandler(a, "jarvis.frame.get", 15*time.Second, func(ctx context.Context, p struct {
			RunID         string `json:"run_id"`
			ObservationID string `json:"observation_id"`
		}) (struct {
			Observation computer.Observation `json:"observation"`
		}, error) {
			o, err := a.frame(ctx, p.RunID, p.ObservationID)
			return struct {
				Observation computer.Observation `json:"observation"`
			}{o}, err
		}),
		"jarvis.evidence.export": asyncHandler(a, "jarvis.evidence.export", 30*time.Second, func(ctx context.Context, p struct {
			RunID       string `json:"run_id"`
			Destination string `json:"destination"`
		}) (struct {
			Path string `json:"path"`
		}, error) {
			path, err := a.export(ctx, p.RunID, p.Destination)
			return struct {
				Path string `json:"path"`
			}{path}, err
		}),
		"jarvis.trace.load": handler(func(_ context.Context, p pathRequest) (json.RawMessage, error) {
			b, err := readBounded(a.resolve(p.Path), 16<<20)
			if err == nil && !json.Valid(b) {
				err = errors.New("invalid trace JSON")
			}
			return json.RawMessage(b), err
		}),
		"jarvis.trace.replay": handler(func(_ context.Context, p struct {
			Path           string                    `json:"path"`
			CapabilityPath string                    `json:"capability_path"`
			Inputs         map[string]workflow.Value `json:"inputs"`
		}) (struct {
			State workflow.State `json:"state"`
		}, error) {
			raw, err := readBounded(a.resolve(p.Path), 16<<20)
			if err != nil {
				return struct {
					State workflow.State `json:"state"`
				}{}, err
			}
			program, err := a.Compile(p.CapabilityPath)
			if err != nil {
				return struct {
					State workflow.State `json:"state"`
				}{}, err
			}
			state, err := offline.Replay(program.Bytes(), raw, p.Inputs)
			return struct {
				State workflow.State `json:"state"`
			}{state}, err
		}),
	}
	for name, h := range ops {
		if err := r.Register(name, h); err != nil {
			return err
		}
	}
	return a.configureDiscovery(r)
}
func (a *App) history(id string) ([]promotion, error) {
	if id == "" || strings.ContainsAny(id, "/\\") || id == ".." {
		return nil, errors.New("invalid catalog ID")
	}
	b, err := readBounded(filepath.Join(a.cfg.Root, ".local", "promotions", id+".json"), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return []promotion{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []promotion
	err = json.Unmarshal(b, &out)
	return out, err
}
func (a *App) Frame(id, observation string) (computer.Observation, error) {
	return a.frame(a.ctx, id, observation)
}
func (a *App) frame(ctx context.Context, id, observation string) (computer.Observation, error) {
	if err := ctx.Err(); err != nil {
		return computer.Observation{}, err
	}
	e, err := a.entry(id)
	if err != nil {
		return computer.Observation{}, err
	}
	e.mu.Lock()
	dir := e.view.EvidenceDir
	e.mu.Unlock()
	files, err := os.ReadDir(dir)
	if err != nil {
		return computer.Observation{}, err
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return computer.Observation{}, err
		}
		if !strings.HasPrefix(f.Name(), "frame-") || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := readBounded(filepath.Join(dir, f.Name()), 1<<20)
		if err != nil {
			return computer.Observation{}, err
		}
		var o computer.Observation
		if err = json.Unmarshal(raw, &o); err != nil {
			return o, err
		}
		if o.ID != observation {
			continue
		}
		png, err := readBounded(filepath.Join(dir, strings.TrimSuffix(f.Name(), ".json")+".png"), 16<<20)
		if err != nil {
			return o, err
		}
		if err := ctx.Err(); err != nil {
			return o, err
		}
		o.Screenshot.Base64 = encodeImage(png)
		return o, nil
	}
	return computer.Observation{}, errors.New("observation frame not found")
}
func (a *App) Export(id, destination string) (string, error) { return a.export(a.ctx, id, destination) }
func (a *App) export(ctx context.Context, id, destination string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	e, err := a.entry(id)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	dir, finished := e.view.EvidenceDir, e.view.Finished
	e.mu.Unlock()
	if !finished {
		return "", errors.New("finish the run before exporting")
	}
	destination = a.resolve(destination)
	if !strings.HasSuffix(destination, ".zip") {
		return "", errors.New("export destination must end in .zip")
	}
	f, err := os.CreateTemp(filepath.Dir(destination), ".jarvis-export-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	z := zip.NewWriter(f)
	files, err := os.ReadDir(dir)
	if err != nil {
		f.Close()
		return "", err
	}
	var total int64
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			z.Close()
			f.Close()
			return "", err
		}
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			z.Close()
			f.Close()
			return "", err
		}
		if !info.Mode().IsRegular() {
			z.Close()
			f.Close()
			return "", errors.New("nonregular evidence export source")
		}
		total += info.Size()
		if total > 300<<20 {
			z.Close()
			f.Close()
			return "", errors.New("export exceeds300 MiB")
		}
		in, err := os.Open(filepath.Join(dir, file.Name()))
		if err != nil {
			z.Close()
			f.Close()
			return "", err
		}
		out, err := z.Create(file.Name())
		if err == nil {
			_, err = io.Copy(out, contextReader{ctx: ctx, reader: io.LimitReader(in, info.Size()+1)})
		}
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			z.Close()
			f.Close()
			return "", fmt.Errorf("export artifact: %w", err)
		}
	}
	if err = z.Close(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Link(f.Name(), destination); err != nil {
		return "", err
	}
	return destination, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
