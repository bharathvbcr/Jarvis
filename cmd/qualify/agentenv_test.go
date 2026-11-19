package main

import "testing"

func TestQualifyRefusesAnAgentEnvironment(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if err := refuseAgentEnvironment(getenv); err != nil {
		t.Fatalf("a person's environment was refused: %v", err)
	}
	for _, mode := range []string{"yolo", "anything"} {
		env["JARVIS_AGENT_MODE"] = mode
		if err := refuseAgentEnvironment(getenv); err == nil {
			t.Fatalf("JARVIS_AGENT_MODE=%q did not refuse qualification", mode)
		}
	}
}
