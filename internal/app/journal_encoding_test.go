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

// The journal encoders must write what encoding/json v1 writes. These cases
// compare them to v1 at run time, so the check does not depend on the
// transcribed constants above, over the inputs where v1 and v2 differ.
func TestJournalEncodersMatchV1(t *testing.T) {
	empty := ""
	for name, mutate := range map[string]func(*computer.Observation){
		"nil slices":           func(o *computer.Observation) { o.Nodes[0].Actions, o.Nodes[0].NativePath = nil, nil },
		"empty non-nil slices": func(o *computer.Observation) { o.Nodes[0].Actions, o.Nodes[0].NativePath = []string{}, []uint32{} },
		"nil nodes":            func(o *computer.Observation) { o.Nodes = nil },
		"empty non-nil nodes":  func(o *computer.Observation) { o.Nodes = []computer.Node{} },
		"invalid UTF-8":        func(o *computer.Observation) { o.Nodes[0].Name = "bad\xff\xfeutf8 \xc3" },
		"html-sensitive":       func(o *computer.Observation) { o.Window.Title = "<script>&amp;</script>" },
		"empty string pointer": func(o *computer.Observation) { o.Nodes[0].Value = &empty },
		"zero-valued flags":    func(o *computer.Observation) { o.Complete, o.PointerMotion = false, false },
		"truncated reason set": func(o *computer.Observation) { o.TruncatedReason, o.PointerMotion = "limit", true },
		"line separators": func(o *computer.Observation) {
			o.Nodes[1].Name = "a" + string(rune(0x2028)) + string(rune(0x2029)) + "b"
		},
		"large integers": func(o *computer.Observation) { o.CapturedAtMillis, o.Epoch = 1<<53+1, 1<<63 },
	} {
		t.Run(name, func(t *testing.T) {
			record := encodingRecord()
			mutate(record.Observation)
			wantLine, err := jsonv1.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			gotLine, err := encodeLine(record)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotLine) != string(wantLine) {
				t.Errorf("events.jsonl line differs from v1:\nv1 %s\nv2 %s", wantLine, gotLine)
			}
			wantTree, err := jsonv1.MarshalIndent(record.Observation, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotTree, err := encodeTree(*record.Observation)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotTree) != string(wantTree) {
				t.Errorf("tree file differs from v1:\nv1 %s\nv2 %s", wantTree, gotTree)
			}
		})
	}
}

// The records carry no map today, so the map cases run on a probe type with the
// same tags: they pin the option set the encoders rely on, which is what keeps
// a map that is added later byte-compatible.
func TestV1OptionSetMatchesV1ForMapsAndEmptyValues(t *testing.T) {
	type probe struct {
		NilMap     map[string]int    `json:"nil_map"`
		EmptyMap   map[string]int    `json:"empty_map"`
		OmitMap    map[string]int    `json:"omit_map,omitempty"`
		FullMap    map[string]string `json:"full_map"`
		NilSlice   []int             `json:"nil_slice"`
		EmptySlice []int             `json:"empty_slice"`
		OmitSlice  []int             `json:"omit_slice,omitempty"`
		OmitZero   int               `json:"omit_zero,omitempty"`
		OmitFalse  bool              `json:"omit_false,omitempty"`
		OmitStruct struct{}          `json:"omit_struct,omitempty"`
		Text       string            `json:"text"`
		Raw        jsonv1.RawMessage `json:"raw"`
	}
	p := probe{EmptyMap: map[string]int{}, OmitMap: map[string]int{}, FullMap: map[string]string{"b": "<>&", "a": "x", "c": "bad\xff"},
		EmptySlice: []int{}, OmitSlice: []int{}, Text: "<>&\xff", Raw: jsonv1.RawMessage(`{"k":  [1, 2]}`)}
	want, err := jsonv1.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jsonv2.Marshal(p, jsonv1.DefaultOptionsV1())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("v2 with the v1 option set differs from v1:\nv1 %s\nv2 %s", want, got)
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
