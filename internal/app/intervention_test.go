package app

import (
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

func TestParseActSpec(t *testing.T) {
	kind, name, text, err := parseActSpec("press Dismiss")
	if err != nil || kind != "press" || name != "Dismiss" || text != nil {
		t.Fatalf("%s %s %v %v", kind, name, text, err)
	}
	kind, name, text, err = parseActSpec("set_value Passcode BRANCH-7741")
	if err != nil || kind != "set_value" || name != "Passcode" || text == nil || *text != "BRANCH-7741" {
		t.Fatalf("%s %s %v %v", kind, name, text, err)
	}
	if _, _, _, err = parseActSpec("press"); err == nil {
		t.Fatal("expected error")
	}
	if _, _, _, err = parseActSpec("type Hello"); err == nil {
		t.Fatal("expected unsupported kind")
	}
}

func TestStuckReasonCodesExported(t *testing.T) {
	if computer.ReasonLadderExhausted == "" || computer.InterventionRequested == "" {
		t.Fatal("intervention constants missing")
	}
}
