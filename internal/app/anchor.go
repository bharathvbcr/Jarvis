package app

import (
	"context"
	"errors"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type AnchorRequest struct {
	RunID         string              `json:"run_id"`
	ObservationID string              `json:"observation_id"`
	Epoch         uint64              `json:"epoch"`
	Crop          workflow.PixelRect  `json:"crop"`
	Search        workflow.PixelRect  `json:"search"`
	Click         workflow.PixelPoint `json:"click"`
}
type AnchorResponse struct {
	Visual workflow.VisualAnchor `json:"visual"`
}

func anchorSnapshot(e *runEntry, req AnchorRequest) (computer.Observation, computer.PrivacyPolicy, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.view.Finished || e.view.RunID != req.RunID {
		return computer.Observation{}, computer.PrivacyPolicy{}, errors.New("anchor requires the current active run")
	}
	o := e.view.Observation
	epoch := e.session.Epoch
	if e.run != nil {
		o = e.run.Observation()
		epoch = e.run.Snapshot().Epoch
	}
	if o == nil || req.ObservationID == "" || req.Epoch == 0 || req.Epoch != epoch || o.Epoch != epoch || o.ID != req.ObservationID {
		return computer.Observation{}, computer.PrivacyPolicy{}, errors.New("anchor requires the current observation and control epoch")
	}
	return *o, e.privacy, nil
}
func (a *App) CreateAnchor(ctx context.Context, req AnchorRequest) (AnchorResponse, error) {
	if err := ctx.Err(); err != nil {
		return AnchorResponse{}, err
	}
	e, err := a.entry(req.RunID)
	if err != nil {
		return AnchorResponse{}, err
	}
	o, policy, err := anchorSnapshot(e, req)
	if err != nil {
		return AnchorResponse{}, err
	}
	visual, err := computer.CreateVisualAnchor(o, policy, req.Crop, req.Search, req.Click)
	if err != nil {
		return AnchorResponse{}, err
	}
	if err = ctx.Err(); err != nil {
		return AnchorResponse{}, err
	}
	if _, _, err = anchorSnapshot(e, req); err != nil {
		return AnchorResponse{}, err
	}
	return AnchorResponse{Visual: visual}, nil
}
