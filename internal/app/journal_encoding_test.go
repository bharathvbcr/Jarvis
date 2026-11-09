package app

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
	"github.com/bharathvbcr/Manvi/manvi/workflow"
)

// unescape turns the ~u marker back into a JSON \u escape. The expected bytes
// are written with the marker because a literal backslash-u in this file is
// too easy to rewrite into the character it names.
func unescape(s string) string { return strings.ReplaceAll(s, "~u", `\u`) }

// encodingRecord exercises every place encoding/json v1 and v2 differ: nil
// slices and maps, zero values under omitempty, HTML-sensitive and non-ASCII
// text, a pointer left nil.
func encodingRecord() computer.Record {
	value := "a<b>&c " + string(rune(0x2028)) + " é"
	return computer.Record{
		Kind: "observation", Stage: "observe", ActionID: "run:step-1", Actor: "automation", Epoch: 3,
		Observation: &computer.Observation{
			ID: "obs-1", Epoch: 3, Complete: true,
			Window: computer.Window{PID: 7, ID: 9, Title: "Bank <main> & co", Foreground: true, Scale: 2},
			Nodes: []computer.Node{
				{ID: "n1", Role: "text_field", Name: "Balance", Value: &value, Enabled: true},
				{ID: "n2", Role: "button", Name: "Send", Actions: []string{"press"}, NativePath: []uint32{0, 1}},
			},
			Screenshot: computer.Screenshot{MIMEType: "image/png", Width: 10, Height: 20},
		},
		Event: &workflow.Event{Kind: "observed", Epoch: 3, Observation: workflow.Observation{ID: "o", Matches: 1}},
	}
}

const wantEncodedLine = `{"kind":"observation","stage":"observe","action_id":"run:step-1","actor":"automation","epoch":3,"observation":{"observation_id":"obs-1","epoch":3,"captured_at_ms":0,"tree_at_ms":0,"window":{"pid":7,"window_id":9,"title":"Bank ~u003cmain~u003e ~u0026 co","process_identity":"","bounds":{"x":0,"y":0,"width":0,"height":0},"foreground":true,"scale":2},"nodes":[{"id":"n1","role":"text_field","name":"Balance","value":"a~u003cb~u003e~u0026c ~u2028 é","enabled":true,"editable":false,"actions":null,"native_path":null},{"id":"n2","role":"button","name":"Send","enabled":false,"editable":false,"actions":["press"],"native_path":[0,1]}],"screenshot":{"mime_type":"image/png","base64":"","width":10,"height":20},"complete":true},"event":{"sequence":0,"run_id":"","session_id":"","epoch":3,"elapsed_millis":0,"kind":"observed","observation":{"id":"o","target_id":"","matches":1,"complete":false,"actionable":false,"value":{"type":""}}}}`

const wantEncodedTree = `{
  "observation_id": "obs-1",
  "epoch": 3,
  "captured_at_ms": 0,
  "tree_at_ms": 0,
  "window": {
    "pid": 7,
    "window_id": 9,
    "title": "Bank ~u003cmain~u003e ~u0026 co",
    "process_identity": "",
    "bounds": {
      "x": 0,
      "y": 0,
      "width": 0,
      "height": 0
    },
    "foreground": true,
    "scale": 2
  },
  "nodes": [
    {
      "id": "n1",
      "role": "text_field",
      "name": "Balance",
      "value": "a~u003cb~u003e~u0026c ~u2028 é",
      "enabled": true,
      "editable": false,
      "actions": null,
      "native_path": null
    },
    {
      "id": "n2",
      "role": "button",
      "name": "Send",
      "enabled": false,
      "editable": false,
      "actions": [
        "press"
      ],
      "native_path": [
        0,
        1
      ]
    }
  ],
  "screenshot": {
    "mime_type": "image/png",
    "base64": "",
    "width": 10,
    "height": 20
  },
  "complete": true
}`

// TestJournalEncodingIsStable pins the bytes of the two journal encoders. The
// verifier hashes frame trees and the journal is the run's durable record, so a
// change in these bytes is a change to the evidence format and has to be a
// decision, not a side effect of a library change.
func TestJournalEncodingIsStable(t *testing.T) {
	line, err := encodeLine(encodingRecord())
	if err != nil {
		t.Fatal(err)
	}
	if want := unescape(wantEncodedLine); string(line) != want {
		t.Fatalf("events.jsonl line changed:\nwant %s\ngot  %s", want, line)
	}
	tree, err := encodeTree(*encodingRecord().Observation)
	if err != nil {
		t.Fatal(err)
	}
	if want := unescape(wantEncodedTree); string(tree) != want {
		t.Fatalf("frame tree file changed:\nwant %s\ngot  %s", want, tree)
	}
}

// BenchmarkJournalEncoding compares the encoders for one observation record of
// 20 nodes (the size of a bank window) and its tree file: encoding/json v1,
// encoding/json/v2 with its own defaults, and v2 with the v1 option set, which
// is what v1 Marshal is in Go 1.27.
func BenchmarkJournalEncoding(b *testing.B) {
	record := costReadStep(1, "", "")[0]
	line := map[string]func() ([]byte, error){
		"v1":        func() ([]byte, error) { return jsonv1.Marshal(record) },
		"v2":        func() ([]byte, error) { return jsonv2.Marshal(record) },
		"v2+v1opts": func() ([]byte, error) { return jsonv2.Marshal(record, jsonv1.DefaultOptionsV1()) },
	}
	tree := map[string]func() ([]byte, error){
		"v1": func() ([]byte, error) { return jsonv1.MarshalIndent(record.Observation, "", "  ") },
		"v2": func() ([]byte, error) { return jsonv2.Marshal(record.Observation, jsontext.WithIndent("  ")) },
		"v2+v1opts": func() ([]byte, error) {
			return jsonv2.Marshal(record.Observation, jsonv1.DefaultOptionsV1(), jsontext.WithIndent("  "))
		},
	}
	for _, kind := range []struct {
		name string
		fns  map[string]func() ([]byte, error)
	}{{"line", line}, {"tree", tree}} {
		for _, impl := range []string{"v1", "v2", "v2+v1opts"} {
			fn := kind.fns[impl]
			b.Run(kind.name+"/"+impl, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := fn(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
