package trace

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

func TestOfflineTraceRejectsForeignAndTamperedState(t *testing.T) {
	raw, err := os.ReadFile("../../examples/balance.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := workflow.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := workflow.NewState(p, "run", "session", 1)
	if err != nil {
		t.Fatal(err)
	}
	event := workflow.Event{Sequence: 1, RunID: "run", SessionID: "session", Epoch: 1, Kind: "cancel"}
	state, _, err := workflow.Reduce(p, initial, event, nil)
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Initial: initial, State: state, Events: []workflow.Event{event}}
	encoded, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Replay(raw, encoded, nil); err != nil {
		t.Fatal(err)
	}
	forged := Journal{Initial: initial, State: initial, Events: []workflow.Event{}}
	forged.Initial.Phase = workflow.Completed
	forged.State.Phase = workflow.Completed
	encoded, _ = json.Marshal(forged)
	if _, err = Replay(raw, encoded, nil); err == nil {
		t.Fatal("forged completed initial state accepted without events")
	}
	encoded, _ = json.Marshal(j)
	encoded = append(encoded[:len(encoded)-1], []byte(`,"passed":true}`)...)
	if _, err = Replay(raw, encoded, nil); err == nil {
		t.Fatal("unrecognized runner verdict accepted")
	}
	j.State.Phase = workflow.Completed
	encoded, _ = json.Marshal(j)
	if _, err = Replay(raw, encoded, nil); err == nil {
		t.Fatal("forged terminal completion accepted")
	}
	j.State = state
	j.Events[0].RunID = "foreign"
	encoded, _ = json.Marshal(j)
	if _, err = Replay(raw, encoded, nil); err == nil {
		t.Fatal("foreign event accepted")
	}
}
