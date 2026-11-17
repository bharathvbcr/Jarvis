package app

import (
	"sort"
	"strings"
)

// redactWatched replaces every occurrence of every watched value in text with
// [redacted]. The matches are all found in the original text and the
// overlapping ones merged before anything is replaced, so the result does not
// depend on the order of watched. Replacing one value at a time does: whichever
// of two overlapping values (abc, abcdef) went first split the other and left a
// piece of it behind, and the order came from a map.
func redactWatched(text string, watched []string) string {
	type span struct{ lo, hi int }
	var spans []span
	for _, w := range watched {
		if w == "" {
			continue
		}
		// Step one byte past each hit, not one value-length, so a value that
		// overlaps itself (aaa in aaaa) is masked in full.
		for from := 0; from < len(text); {
			i := strings.Index(text[from:], w)
			if i < 0 {
				break
			}
			spans = append(spans, span{from + i, from + i + len(w)})
			from += i + 1
		}
	}
	if len(spans) == 0 {
		return text
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].lo < spans[b].lo })
	var out strings.Builder
	at := 0
	for i := 0; i < len(spans); {
		lo, hi := spans[i].lo, spans[i].hi
		for i++; i < len(spans) && spans[i].lo < hi; i++ {
			hi = max(hi, spans[i].hi)
		}
		out.WriteString(text[at:lo])
		out.WriteString("[redacted]")
		at = hi
	}
	out.WriteString(text[at:])
	return out.String()
}
