package technical

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScriptResearcher(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "research.sh")

		err := os.WriteFile(
			scriptPath,
			[]byte(`#!/bin/sh
echo '{"symbol":"600519.SH","requested_as_of":"20260923","status":"NORMAL"}'
`),
			0700,
		)
		if err != nil {
			t.Fatal(err)
		}

		researcher, err := NewScriptResearcher(
			ScriptResearcherConfig{
				Python:     "sh",
				ScriptPath: scriptPath,
			},
		)
		if err != nil {
			t.Fatalf(
				"NewScriptResearcher() error = %v",
				err,
			)
		}

		result, err := researcher.Research(
			context.Background(),
			Request{
				Symbol: "600519.SH",
				AsOf:   "20260923",
			},
		)
		if err != nil {
			t.Fatalf(
				"Research() error = %v",
				err,
			)
		}

		if result["symbol"] != "600519.SH" {
			t.Fatalf(
				"symbol = %v",
				result["symbol"],
			)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		scriptPath := filepath.Join(dir, "research.sh")

		err := os.WriteFile(
			scriptPath,
			[]byte(`#!/bin/sh
echo 'not json'
`),
			0700,
		)
		if err != nil {
			t.Fatal(err)
		}

		researcher, err := NewScriptResearcher(
			ScriptResearcherConfig{
				Python:     "sh",
				ScriptPath: scriptPath,
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = researcher.Research(
			context.Background(),
			Request{
				Symbol: "600519.SH",
				AsOf:   "20260923",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing script", func(t *testing.T) {
		_, err := NewScriptResearcher(
			ScriptResearcherConfig{
				ScriptPath: filepath.Join(
					t.TempDir(),
					"missing.py",
				),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
