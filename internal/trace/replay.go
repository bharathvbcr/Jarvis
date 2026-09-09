// Package trace reconstructs deterministic state without a desktop or provider.
package trace

import (
	"errors"
	"reflect"

	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type Journal struct {
	Initial workflow.State   `json:"initial"`
	State   workflow.State   `json:"state"`
	Events  []workflow.Event `json:"events"`
	Error   string           `json:"error,omitempty"`
}

func Replay(capability, raw []byte, inputs map[string]workflow.Value) (workflow.State, error) {
	p, err := workflow.Compile(capability)
	if err != nil {
		return workflow.State{}, err
	}
	if len(raw) > 16<<20 {
		return workflow.State{}, errors.New("trace exceeds16 MiB")
	}
	var j Journal
	if err = workflow.DecodeStrict(raw, &j); err != nil {
		return workflow.State{}, err
	}
	initial, err := workflow.NewState(p, j.Initial.RunID, j.Initial.SessionID, j.Initial.Epoch)
	if err != nil {
		return workflow.State{}, err
	}
	if !reflect.DeepEqual(initial, j.Initial) {
		return workflow.State{}, errors.New("trace initial state is not canonical admission")
	}
	if len(j.Events) > 4096 {
		return workflow.State{}, errors.New("trace event limit exceeded")
	}
	state, err := workflow.Replay(p, j.Initial, j.Events, inputs)
	if err != nil {
		return state, err
	}
	if !reflect.DeepEqual(state, j.State) {
		return state, errors.New("reconstructed state disagrees with recorded terminal state")
	}
	return state, nil
}
