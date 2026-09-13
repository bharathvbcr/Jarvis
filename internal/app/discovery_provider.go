package app

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bharathvbcr/Manvi/manvi/credentials"
	"github.com/bharathvbcr/Manvi/manvi/llm"
	"github.com/bharathvbcr/Manvi/manvi/llm/budget"
	"github.com/bharathvbcr/Manvi/manvi/llm/gemini"
	"github.com/bharathvbcr/Manvi/manvi/llm/replay"
)

const (
	DiscoveryProviderGemini  = "gemini"
	DiscoveryProviderFixture = "fixture"
	DiscoveryEvidenceLive    = "live"
	DiscoveryEvidenceNotLive = "not live"
)

// defaultDiscoveryFixture is the recorded tool transcript used by --provider
// fixture. It is shaped like Manvi's Gemini mock-wire / replay fixtures: ordered
// model turns with tool calls, so the discovery loop can be exercised without
// GEMINI_API_KEY.
//
// It is embedded rather than located at runtime. Deriving the path from
// runtime.Caller worked under `go test` but not in the shipped binary, which is
// built with -trimpath: the recorded file name is then the module-relative path,
// and joining it to the working directory names a file that does not exist.
//
//go:embed testdata/discovery/balance-tool-transcript.json
var defaultDiscoveryFixture []byte

const defaultDiscoveryFixtureName = "embedded:balance-tool-transcript.json"

func normalizeDiscoveryProvider(name string) (string, error) {
	switch name {
	case "", DiscoveryProviderGemini:
		return DiscoveryProviderGemini, nil
	case DiscoveryProviderFixture:
		return DiscoveryProviderFixture, nil
	default:
		return "", fmt.Errorf("provider must be %s or %s", DiscoveryProviderGemini, DiscoveryProviderFixture)
	}
}

func discoveryEvidenceClass(provider string) string {
	if provider == DiscoveryProviderFixture {
		return DiscoveryEvidenceNotLive
	}
	return DiscoveryEvidenceLive
}

func discoveryLive(provider string) bool {
	return provider == DiscoveryProviderGemini
}

// resolveDiscoveryFixture validates an explicitly supplied fixture path. An
// empty path means the embedded transcript, which needs no resolution.
func resolveDiscoveryFixture(path string) (string, error) {
	if path == "" {
		return "", errors.New("fixture path unavailable")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("discovery fixture: %w", err)
	}
	return abs, nil
}

func newDiscoveryLLMProvider(provider, fixturePath string, resolveGemini func() (credentials.Secret, error), ledger *budget.Ledger) (llm.Provider, string, error) {
	kind, err := normalizeDiscoveryProvider(provider)
	if err != nil {
		return nil, "", err
	}
	switch kind {
	case DiscoveryProviderFixture:
		if fixturePath == "" {
			inner, err := replay.Decode(defaultDiscoveryFixture, defaultDiscoveryFixtureName)
			if err != nil {
				return nil, "", err
			}
			// Replay fixtures are not AttemptGated; budget wrapping belongs only to live Gemini HTTP.
			return inner, defaultDiscoveryFixtureName, nil
		}
		path, err := resolveDiscoveryFixture(fixturePath)
		if err != nil {
			return nil, "", err
		}
		inner, err := replay.Load(path)
		if err != nil {
			return nil, "", err
		}
		return inner, path, nil
	default:
		return &budget.Provider{Inner: gemini.New("", resolveGemini), Ledger: ledger}, "", nil
	}
}
