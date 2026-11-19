package pullback

import (
	"errors"
	"testing"
)

func TestV1DefinitionFingerprint(t *testing.T) {
	const v1 = "trend-pullback-v1"
	const expected = "38c8ffa6c6db0a86513c0709fc3ccf39e47e94a0c66c2bd9ab391877d65e0c6e"
	definition, err := Definition(v1)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := definition.SHA256()
	if err != nil || digest != expected {
		t.Fatalf("v1 definition changed: %s, %v; register a new version for new parameters", digest, err)
	}
	definition.FastSessions = 10
	fresh, err := Definition(v1)
	if err != nil || fresh.FastSessions != 20 {
		t.Fatalf("registered definition mutated: %+v, %v", fresh, err)
	}
	if _, err := ScanVersion("unknown", "20261113", nil); !errors.Is(err, ErrUnknownStrategy) {
		t.Fatalf("unknown versions must fail closed: %v", err)
	}
}
