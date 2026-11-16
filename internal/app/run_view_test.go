package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

// fillNonZero populates every settable field reachable from v with a non-zero
// value: slices get two elements, maps one entry, pointers a target. A run view
// filled this way has something to share at every place a caller could write.
func fillNonZero(v reflect.Value, path string) { fillDepth(v, path, 0) }

// fillDepth stops allocating below depth 6: workflow.Selector contains a slice
// of itself, so an unbounded fill never ends.
func fillDepth(v reflect.Value, path string, depth int) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("s:" + path)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Pointer:
		if depth < 6 {
			v.Set(reflect.New(v.Type().Elem()))
			fillDepth(v.Elem(), path, depth+1)
		}
	case reflect.Slice:
		if v.Type() == reflect.TypeOf(json.RawMessage(nil)) {
			v.Set(reflect.ValueOf(json.RawMessage(`"raw"`)))
		} else if depth < 6 {
			v.Set(reflect.MakeSlice(v.Type(), 2, 2))
			for i := 0; i < 2; i++ {
				fillDepth(v.Index(i), path, depth+1)
			}
		}
	case reflect.Map:
		if depth < 6 {
			v.Set(reflect.MakeMap(v.Type()))
			key := reflect.New(v.Type().Key()).Elem()
			fillDepth(key, path+"/key", depth+1)
			elem := reflect.New(v.Type().Elem()).Elem()
			fillDepth(elem, path+"/val", depth+1)
			v.SetMapIndex(key, elem)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				fillDepth(f, path+"."+v.Type().Field(i).Name, depth)
			}
		}
	}
}

// scribble writes through every pointer, slice element and map entry reachable
// from v, the way a hostile or careless caller of Get could.
func scribble(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			v.SetString(v.String() + "!MUTATED")
		}
	case reflect.Bool:
		if v.CanSet() {
			v.SetBool(!v.Bool())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.CanSet() {
			v.SetInt(v.Int() + 100)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.CanSet() {
			v.SetUint(v.Uint() + 100)
		}
	case reflect.Float32, reflect.Float64:
		if v.CanSet() {
			v.SetFloat(v.Float() + 100)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			scribble(v.Elem())
		}
	case reflect.Slice:
		if v.Type() == reflect.TypeOf(json.RawMessage(nil)) {
			// Write in place, but keep the document valid JSON.
			if v.Len() > 1 {
				v.Index(1).SetUint('X')
			}
			return
		}
		for i := 0; i < v.Len(); i++ {
			scribble(v.Index(i))
		}
	case reflect.Map:
		iter := v.MapRange()
		type entry struct{ k, v reflect.Value }
		var entries []entry
		for iter.Next() {
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(iter.Value())
			scribble(e)
			entries = append(entries, entry{iter.Key(), e})
		}
		for _, e := range entries {
			v.SetMapIndex(e.k, e.v)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				scribble(f)
			}
		}
	}
}

func mustJSON(tb testing.TB, v any) string {
	tb.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		tb.Fatal(err)
	}
	return string(raw)
}

// populatedEntry is a run entry whose view has every field set, so no part of
// the view escapes the mutation tests by being empty.
func populatedEntry(tb testing.TB) (*App, *runEntry) {
	tb.Helper()
	a, e := costEntry(tb, 0, 0)
	var view RunView
	fillNonZero(reflect.ValueOf(&view).Elem(), "view")
	view.RunID = "run"
	view.State.StepIndex = 0
	e.view = view
	a.runs["run"] = e
	return a, e
}

// A caller of Get must not be able to change the run through anything in the
// view it receives: not a record, not a node, not a map or a pointer target.
func TestGetViewCannotMutateRunState(t *testing.T) {
	a, e := populatedEntry(t)
	before := mustJSON(t, e.view)
	first, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	pristine := mustJSON(t, first)
	scribble(reflect.ValueOf(&first).Elem())
	if mustJSON(t, first) == pristine {
		t.Fatal("scribble changed nothing: the view has nothing writable, so this test proves nothing")
	}
	if after := mustJSON(t, e.view); after != before {
		t.Fatalf("mutating the returned view changed the run's state:\nbefore %.300s\nafter  %.300s", before, after)
	}
	second, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustJSON(t, second); got != pristine {
		t.Fatalf("a later Get saw the earlier caller's writes:\nwant %.300s\ngot  %.300s", pristine, got)
	}
}

// onRecord hands the run loop's record to the view; the loop keeps using that
// record, so the stored copy must not alias it.
func TestAppendRecordStoresACopyTheCallerCannotReach(t *testing.T) {
	_, e := costEntry(t, 0, 0)
	var record computer.Record
	fillNonZero(reflect.ValueOf(&record).Elem(), "record")
	record.Observation.Screenshot.Base64 = costFrame(64<<10, 4)
	if err := e.appendRecord(record); err != nil {
		t.Fatal(err)
	}
	stored := mustJSON(t, e.view.Records)
	scribble(reflect.ValueOf(&record).Elem())
	if got := mustJSON(t, e.view.Records); got != stored {
		t.Fatalf("mutating the caller's record changed the stored record:\nwant %.300s\ngot  %.300s", stored, got)
	}
	if e.view.Records[0].Observation.Screenshot.Base64 != "" {
		t.Fatal("the run view kept a frame")
	}
	if record.Observation.Screenshot.Base64 == "" {
		t.Fatal("appendRecord blanked the caller's frame; the journal and the run loop still need it")
	}
}

// TestGetAllocationDoesNotScaleWithTheFrame pins the per-poll cost: a poll may
// not encode, decode or copy the live frame. The frame is a string and strings
// are immutable, so sharing it is safe.
func TestGetAllocationDoesNotScaleWithTheFrame(t *testing.T) {
	const frame = 1500 << 10
	a, _ := costEntry(t, 10, frame)
	got := allocatedBytes(20, func() {
		if _, err := a.Get("run"); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("Get() of 10 records with a %d KB frame allocates %d bytes", frame>>10, got)
	if got > frame/4 {
		t.Fatalf("one Get() poll allocates %d bytes with a %d byte frame in the view: the frame is being re-encoded", got, frame)
	}
}

func TestAppendRecordAllocationDoesNotScaleWithTheFrame(t *testing.T) {
	const frame = 1500 << 10
	_, e := costEntry(t, 0, 0)
	record := computer.Record{Kind: "observation", Stage: "observe", ActionID: "run:step", Actor: "automation", Epoch: 1, Observation: costObservation("obs", costFrame(frame, 3), 20)}
	got := allocatedBytes(20, func() {
		if err := e.appendRecord(record); err != nil {
			t.Fatal(err)
		}
		e.view.Records = e.view.Records[:0]
	})
	t.Logf("appendRecord of a record with a %d KB frame allocates %d bytes", frame>>10, got)
	if got > frame/4 {
		t.Fatalf("one onRecord copy allocates %d bytes with a %d byte frame: the frame is being round-tripped through JSON", got, frame)
	}
}
