package app

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bharathvbcr/Manvi/manvi/gussetcheck"
)

func TestGussetStatus(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{gussetcheck.ErrNotLinked, "not_linked"},
		{fmt.Errorf("wrapped: %w", gussetcheck.ErrNotLinked), "not_linked"},
		{errors.New("poisoned"), "failed: poisoned"},
	} {
		if got := gussetStatus(c.err); got != c.want {
			t.Errorf("gussetStatus(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}
