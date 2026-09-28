//go:build cgo

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestGussetCheckWithTheEngineLinked(t *testing.T) {
	var out bytes.Buffer
	if err := gussetCheck(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gusset-check: ok") {
		t.Fatalf("output %q", out.String())
	}
}
