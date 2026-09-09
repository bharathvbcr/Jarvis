package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestAnchorUsesTrustedCurrentFrameAndRejectsPrivateOrStaleRequests(t *testing.T) {
	s, policy, state, o := reconciliationFixture(t)
	o.Window.Scale = 1
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x * y), A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	o.Screenshot.Base64 = base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	a, err := New(context.Background(), Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	state.Phase = workflow.Paused
	e := &runEntry{session: s, privacy: policy, view: RunView{RunID: "run", State: state, Observation: &o}}
	a.runs["run"] = e
	req := AnchorRequest{RunID: "run", ObservationID: "after", Epoch: 1, Crop: workflow.PixelRect{X: 60, Y: 40, Width: 8, Height: 8}, Search: workflow.PixelRect{X: 60, Y: 40, Width: 16, Height: 16}, Click: workflow.PixelPoint{X: 4, Y: 4}}
	created, err := a.CreateAnchor(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err = created.Visual.Validate(); err != nil {
		t.Fatal(err)
	}
	if created.Visual.FrameWidth != 100 || created.Visual.WindowWidth != 100 || len(created.Visual.SHA256) != 64 {
		t.Fatal("template lost trusted capture geometry or identity")
	}
	for _, kind := range []string{"wrong_run", "old_observation", "old_epoch", "finished", "private_crop", "private_search", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			r := req
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "wrong_run":
				r.RunID = "other"
			case "old_observation":
				r.ObservationID = "before"
			case "old_epoch":
				r.Epoch = 2
			case "finished":
				e.view.Finished = true
				defer func() { e.view.Finished = false }()
			case "private_crop":
				r.Crop = workflow.PixelRect{X: 10, Y: 10, Width: 8, Height: 8}
				r.Search = r.Crop
			case "private_search":
				r.Search = workflow.PixelRect{Width: 100, Height: 80}
			case "cancelled":
				cancel()
			}
			if _, err := a.CreateAnchor(ctx, r); err == nil {
				t.Fatal("unsafe anchor admitted")
			}
		})
	}
}
