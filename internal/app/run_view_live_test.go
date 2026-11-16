package app

import (
	"reflect"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// sharedLive is a live run whose snapshot hands out the very maps, slices and
// pointers it holds. Manvi's Run copies some of these (Outputs) and not others
// (RecoveryCounts); Get must not depend on which, so this source copies none.
type sharedLive struct {
	State workflow.State
	Obs   *computer.Observation
	Ctl   string
	Ivs   []computer.InterventionRequest
}

func (s *sharedLive) Snapshot() workflow.State                      { return s.State }
func (s *sharedLive) Observation() *computer.Observation            { return s.Obs }
func (s *sharedLive) Controller() string                            { return s.Ctl }
func (s *sharedLive) Interventions() []computer.InterventionRequest { return s.Ivs }

// A caller of Get must not be able to change a LIVE run through the view: the
// live state, observation and interventions are copied last, whatever the run
// itself copies.
func TestGetViewCannotMutateLiveRun(t *testing.T) {
	a, e := populatedEntry(t)
	live := &sharedLive{}
	fillNonZero(reflect.ValueOf(&live.State).Elem(), "state")
	fillNonZero(reflect.ValueOf(&live.Ivs).Elem(), "interventions")
	live.Obs = costObservation("live", costFrame(4<<10, 5), 5)
	live.Ctl = "automation"
	live.State.StepIndex = 0
	e.liveSource = live

	before := mustJSON(t, live)
	view, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.State.RecoveryCounts) == 0 || len(view.Interventions) == 0 || view.Observation == nil {
		t.Fatal("the view carries none of the live run's maps and slices, so this test proves nothing")
	}
	pristine := mustJSON(t, view)
	scribble(reflect.ValueOf(&view).Elem())
	if mustJSON(t, view) == pristine {
		t.Fatal("scribble changed nothing")
	}
	if after := mustJSON(t, live); after != before {
		t.Fatalf("mutating the returned view changed the live run:\nbefore %.300s\nafter  %.300s", before, after)
	}
	again, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustJSON(t, again); got != pristine {
		t.Fatalf("a later Get saw the earlier caller's writes:\nwant %.300s\ngot  %.300s", pristine, got)
	}
}
