package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/computer"
)

// ownedCopy copies exported fields only. If RunView or a record ever gains an
// unexported field that can alias memory, the copy would leak run state again,
// so the build must fail rather than the run.
func TestOwnedCopyTypesHaveNoHiddenReferences(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type, path string)
	walk = func(typ reflect.Type, path string) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem(), path)
		case reflect.Map:
			walk(typ.Key(), path)
			walk(typ.Elem(), path)
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if !f.IsExported() {
					if holdsReference(f.Type) {
						t.Errorf("%s.%s is unexported and can alias memory; ownedCopy cannot copy it", path, f.Name)
					}
					continue
				}
				walk(f.Type, path+"."+f.Name)
			}
		case reflect.Interface:
			t.Errorf("%s is an interface; ownedCopy cannot know what it holds", path)
		}
	}
	walk(reflect.TypeOf(RunView{}), "RunView")
	walk(reflect.TypeOf(computer.Record{}), "Record")
}

func TestOwnedCopyPreservesTheValue(t *testing.T) {
	var view RunView
	fillNonZero(reflect.ValueOf(&view).Elem(), "view")
	if got := ownedCopy(view); !reflect.DeepEqual(got, view) {
		t.Fatalf("copy differs from the original:\nwant %.300s\ngot  %.300s", mustJSON(t, view), mustJSON(t, got))
	}
	// nil and empty are different values to a caller that checks, and to JSON.
	empty := RunView{Records: []computer.Record{}}
	if got := ownedCopy(empty); got.Records == nil || ownedCopy(RunView{}).Records != nil {
		t.Fatal("copy changed nil-ness of Records")
	}
}

func TestGetSinceReturnsOnlyTheRecordsAfterTheCursor(t *testing.T) {
	a, _ := costEntry(t, 100, 80<<10)
	full, err := a.Get("run")
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Records) != 100 || full.TotalRecords != 100 {
		t.Fatalf("Get returned %d records, total %d; want 100", len(full.Records), full.TotalRecords)
	}
	tail, err := a.GetSince("run", 90)
	if err != nil {
		t.Fatal(err)
	}
	if tail.TotalRecords != 100 || !reflect.DeepEqual(tail.Records, full.Records[90:]) {
		t.Fatalf("GetSince(90) returned %d records, total %d", len(tail.Records), tail.TotalRecords)
	}
	none, err := a.GetSince("run", 100)
	if err != nil || len(none.Records) != 0 || none.TotalRecords != 100 {
		t.Fatalf("GetSince at the end: %d records, total %d, err %v", len(none.Records), none.TotalRecords, err)
	}
	for _, cursor := range []int{-1, 101} {
		if _, err = a.GetSince("run", cursor); !errors.Is(err, errRecordCursor) {
			t.Fatalf("GetSince(%d) error = %v, want the cursor error", cursor, err)
		}
	}
	tail.Records[0].Kind = "mutated"
	again, err := a.GetSince("run", 90)
	if err != nil || again.Records[0].Kind == "mutated" {
		t.Fatal("a tail view shares its records with the run")
	}
}

// A poller that keeps its cursor pays for what is new, not for the run so far.
func TestGetSinceCostDoesNotGrowWithHistory(t *testing.T) {
	a, _ := costEntry(t, 1000, 80<<10)
	whole := allocatedBytes(10, func() {
		if _, err := a.Get("run"); err != nil {
			t.Fatal(err)
		}
	})
	poll := allocatedBytes(10, func() {
		if _, err := a.GetSince("run", 1000); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("1000 records: Get allocates %d bytes, GetSince at the end %d bytes", whole, poll)
	if poll*10 > whole {
		t.Fatalf("a caught-up poll allocates %d bytes against %d for the whole view", poll, whole)
	}
}
