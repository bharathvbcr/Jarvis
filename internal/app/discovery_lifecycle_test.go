package app

import (
	"context"
	"errors"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
	"testing"
)

func TestDiscoveryRetirementPreservesUnknownAndDenial(t *testing.T) {
	for _, phase := range []workflow.Phase{workflow.Unknown, workflow.Cancelled} {
		v := RunView{State: workflow.State{Phase: phase}}
		finishDiscoveryView(&v, errors.New("provider cancelled after step stopped"))
		if v.State.Phase != phase || !v.Finished {
			t.Fatalf("lost executed step disposition: %+v", v)
		}
	}
}

func TestExecutedDiscoveryFailureStopsFurtherProviderDecisions(t *testing.T) {
	for _, phase := range []workflow.Phase{workflow.Unknown, workflow.Cancelled, workflow.Failed, workflow.Paused, workflow.Completed} {
		ctx, cancel := context.WithCancel(context.Background())
		err := stopDiscoveryAfterStep(cancel, computer.RunResult{State: workflow.State{Phase: phase}}, nil)
		if phase == workflow.Completed {
			if err != nil || ctx.Err() != nil {
				t.Fatal("completed discovery step was stopped")
			}
		} else if err == nil || ctx.Err() != context.Canceled {
			t.Fatalf("%s allowed another model decision", phase)
		}
		cancel()
	}
}
