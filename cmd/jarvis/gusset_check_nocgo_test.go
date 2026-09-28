//go:build !cgo

package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestGussetCheckReportsNotLinkedInTheShippedBuild(t *testing.T) {
	var out bytes.Buffer
	err := gussetCheck(context.Background(), &out)
	if !errors.Is(err, errGussetNotLinked) {
		t.Fatalf("gussetCheck() = %v, want errGussetNotLinked", err)
	}
	if out.Len() != 0 {
		t.Fatalf("a check that could not run printed %q", out.String())
	}
}
