package app

import (
	"math/rand"
	"strings"
	"testing"
)

// The discovery log used to be scrubbed by replacing the watched values one at a
// time, in the order a map happened to yield them. Whichever of two overlapping
// values went first split the other and left a piece of it in the log.
func TestRedactWatchedLeavesNoFragmentOfOverlappingValues(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		watched []string
		want    string
	}{
		{"a value inside another", "id=abcdef;", []string{"abc", "abcdef"}, "id=[redacted];"},
		{"the same pair in the other order", "id=abcdef;", []string{"abcdef", "abc"}, "id=[redacted];"},
		{"values that overlap without nesting", "x abcdefgh y", []string{"abcdef", "cdefgh"}, "x [redacted] y"},
		{"a value that appears twice", "abc and abc", []string{"abc"}, "[redacted] and [redacted]"},
		{"a value repeated against itself", "aaaa", []string{"aaa"}, "[redacted]"},
		{"nothing watched leaves the text alone", "plain text", nil, "plain text"},
		{"an empty watched value is ignored", "plain text", []string{"", "zzz"}, "plain text"},
		{"separate values stay separate", "a-1 b-2", []string{"a-1", "b-2"}, "[redacted] [redacted]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactWatched(c.text, c.watched); got != c.want {
				t.Fatalf("redactWatched(%q, %q) = %q, want %q", c.text, c.watched, got, c.want)
			}
		})
	}
}

// Order must not matter: every permutation of the watched values gives one result.
func TestRedactWatchedIsIndependentOfValueOrder(t *testing.T) {
	watched := []string{"abc", "abcdef", "cdefgh", "ef", "h-9"}
	text := "p=abcdefgh-9 q=abc r=ef"
	want := redactWatched(text, watched)
	for _, secret := range watched {
		for _, w := range strings.Split(want, "[redacted]") {
			if w != "" && strings.Contains(w, secret) {
				t.Fatalf("%q survives in %q", secret, want)
			}
		}
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		shuffled := append([]string(nil), watched...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := redactWatched(text, shuffled); got != want {
			t.Fatalf("order %q gave %q, want %q", shuffled, got, want)
		}
	}
}
