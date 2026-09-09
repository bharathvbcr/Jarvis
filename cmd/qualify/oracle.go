package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bharathvbcr/Jarvis/internal/app"
	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

type oracleExpectation struct{ CreatedName string }

func expectation(scenario string, approved bool) oracleExpectation {
	if approved && (scenario == "creation" || scenario == "delay" || scenario == "crash-after-commit") {
		return oracleExpectation{"Qualification"}
	}
	return oracleExpectation{}
}
func (e oracleExpectation) description() string {
	if e.CreatedName == "" {
		return "seeded balances, members and subaccounts unchanged"
	}
	return fmt.Sprintf("exactly one Savings subaccount %q for M-1001; both seeded balances and member M-1002 unchanged", e.CreatedName)
}

type savedAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type savedMember struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Balance     uint64         `json:"balance_cents"`
	Currency    string         `json:"currency"`
	Subaccounts []savedAccount `json:"subaccounts"`
}
type savedState struct {
	SchemaVersion int           `json:"schema_version"`
	Tenant        string        `json:"tenant"`
	Revision      uint64        `json:"revision"`
	Members       []savedMember `json:"members"`
}

func oracleWithExpectation(path, tenant string, want oracleExpectation) error {
	state, err := readOracle(path)
	if err != nil {
		return err
	}
	return compareOracle(state, tenant, want)
}

// Evaluate both claims against the same bounded saved-state read. A fault may
// behave as expected while the requested business task still failed.
func scenarioOracles(path, tenant, scenario string, approved bool) (faultErr, taskErr error) {
	state, err := readOracle(path)
	if err != nil {
		return err, err
	}
	faultErr = compareOracle(state, tenant, expectation(scenario, approved))
	taskErr = faultErr
	if scenario == "false-ack" && approved {
		taskErr = compareOracle(state, tenant, oracleExpectation{"Qualification"})
	}
	return faultErr, taskErr
}

func readOracle(path string) (savedState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return savedState{}, err
	}
	if !info.Mode().IsRegular() {
		return savedState{}, errors.New("oracle state must be a regular file, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return savedState{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return savedState{}, err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return savedState{}, errors.New("oracle state changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return savedState{}, err
	}
	if len(raw) > 1<<20 {
		return savedState{}, oracleMismatch{"oracle state too large"}
	}
	var state savedState
	if err = workflow.DecodeStrict(raw, &state); err != nil {
		return savedState{}, oracleMismatch{err.Error()}
	}
	return state, nil
}

func compareOracle(state savedState, tenant string, want oracleExpectation) error {
	revision := uint64(0)
	if want.CreatedName != "" {
		revision = 1
	}
	if state.SchemaVersion != 1 || state.Tenant != tenant || len(state.Members) != 2 || state.Revision != revision {
		return oracleMismatch{"saved state differs from independent schema/tenant/revision contract"}
	}
	expected := map[string]savedMember{"M-1001": {ID: "M-1001", Name: "Alex Morgan", Balance: 125000, Currency: "USD"}, "M-1002": {ID: "M-1002", Name: "Jordan Lee", Balance: 84050, Currency: "USD"}}
	for _, member := range state.Members {
		seed, ok := expected[member.ID]
		if !ok || member.Balance != seed.Balance || member.Name != seed.Name || member.Currency != seed.Currency {
			return oracleMismatch{"saved member identity, currency or balance disagrees with the independent seed"}
		}
		if member.ID == "M-1001" && want.CreatedName != "" {
			if len(member.Subaccounts) != 1 || member.Subaccounts[0] != (savedAccount{ID: "S-M-1001-1", Name: want.CreatedName, Kind: "savings"}) {
				return oracleMismatch{"saved creation is missing, duplicated, misnamed, or belongs to a different account type"}
			}
		} else if len(member.Subaccounts) != 0 {
			return oracleMismatch{"unexpected saved subaccount mutation"}
		}
		delete(expected, member.ID)
	}
	if len(expected) != 0 {
		return oracleMismatch{"missing saved member"}
	}
	return nil
}

func retryCounts(records []computer.Record) (observations, inputs int) {
	for _, record := range records {
		if event := record.Event; event != nil {
			if event.Kind == "observe_failed" {
				observations++
			}
			if event.Kind == "receipt" && event.Delivery == "not_sent" && (event.FailureCode == "window_changed" || event.FailureCode == "state_changed") {
				inputs++
			}
		}
	}
	return
}
func approvedInputReached(records []computer.Record, ids []string) bool {
	return len(approvedDispatches(records, ids)) > 0
}
func approvedDispatches(records []computer.Record, ids []string) map[string]int {
	approved := map[string]bool{}
	started := map[string]bool{}
	dispatched := map[string]int{}
	for _, id := range ids {
		approved[id] = true
	}
	for index, record := range records {
		if record.Kind == "action_started" && approved[record.ActionID] {
			started[record.ActionID] = true
		}
		if event := record.Event; event != nil && event.Kind == "receipt" && started[event.ActionID] && (event.Delivery == "sent" || event.Delivery == "unknown") {
			dispatched[event.ActionID] = index
		}
	}
	return dispatched
}
func postStatusObserved(records []computer.Record, dispatched map[string]int, expected string) bool {
	for index, record := range records {
		prior, ok := dispatched[record.ActionID]
		if !ok || prior >= index || record.Stage != "observe_after" || record.Observation == nil || !record.Observation.Complete || record.Epoch != record.Observation.Epoch || record.Epoch != records[prior].Epoch {
			continue
		}
		count, matched := 0, false
		for _, node := range record.Observation.Nodes {
			if node.Name == "Status" && node.Role == "text_field" {
				count++
				matched = node.Value != nil && *node.Value == expected
			}
		}
		if count == 1 && matched {
			return true
		}
	}
	return false
}
func expectedBehavior(scenario string, view app.RunView, item *result, oracleErr, taskOracleErr error) bool {
	if oracleErr != nil || !view.Finished {
		return false
	}
	switch scenario {
	case "balance":
		return item.Outcome == "passed"
	case "creation", "delay":
		return item.Outcome == "passed" && len(item.ApprovedActionIDs) > 0 && (scenario != "delay" || item.FaultReached)
	case "denial":
		return item.ApprovalDenied && len(item.ApprovedActionIDs) == 0 && view.State.Phase == workflow.Cancelled && item.Verdict != "passed"
	case "overlay", "missing-control", "duplicate-control":
		return item.FaultReached && len(item.ApprovedActionIDs) == 0 && (view.State.Phase == workflow.Cancelled || view.State.Phase == workflow.Failed) && item.Verdict != "passed"
	case "commit-noop":
		item.FaultReached = postStatusObserved(view.Records, approvedDispatches(view.Records, item.ApprovedActionIDs), "Creation did not complete")
		return item.FaultReached && view.State.Phase != workflow.Completed && item.Verdict != "passed"
	case "false-ack":
		dispatched := approvedDispatches(view.Records, item.ApprovedActionIDs)
		for id, index := range dispatched {
			if view.Records[index].Event.Delivery != "sent" || view.Records[index].Epoch == 0 {
				delete(dispatched, id)
			}
		}
		item.UIAcknowledgmentObserved = postStatusObserved(view.Records, dispatched, "Subaccount created")
		var mismatch oracleMismatch
		item.FaultReached = item.UIAcknowledgmentObserved && errors.As(taskOracleErr, &mismatch)
		return !item.ApprovalDenied && item.FaultReached && view.State.Phase == workflow.Completed && item.Verdict == "passed" && item.Outcome == "failed" && item.Error == ""
	case "crash-after-commit":
		item.FaultReached = approvedInputReached(view.Records, item.ApprovedActionIDs) && len(item.ApprovedActionIDs) > 0 && view.State.Phase == workflow.Unknown
		return item.FaultReached && item.Verdict != "passed"
	}
	return false
}
