package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	rt, err := New(
		context.Background(),
		Config{
			SkillsDir: t.TempDir(),
			TimeZone:  "Asia/Shanghai",
		},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	handlers := rt.Handlers()
	if len(handlers) != 3 {
		t.Fatalf(
			"Handlers() len = %d, want 3",
			len(handlers),
		)
	}
}

func TestHandlersReturnsCopy(t *testing.T) {
	rt, err := New(
		context.Background(),
		Config{
			SkillsDir: t.TempDir(),
			TimeZone:  "Asia/Shanghai",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	first := rt.Handlers()
	if len(first) == 0 {
		t.Fatal("runtime handlers are empty")
	}

	first[0] = nil

	second := rt.Handlers()
	if second[0] == nil {
		t.Fatal("Handlers() exposed internal handler slice")
	}
}

func TestNewInvalidConfig(t *testing.T) {
	t.Run("missing skills directory", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				TimeZone: "Asia/Shanghai",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("skills path does not exist", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				SkillsDir: filepath.Join(
					t.TempDir(),
					"missing",
				),
				TimeZone: "Asia/Shanghai",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("skills path is file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "skills")

		if err := os.WriteFile(
			path,
			[]byte("not a directory"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		_, err := New(
			context.Background(),
			Config{
				SkillsDir: path,
				TimeZone:  "Asia/Shanghai",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing timezone", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				SkillsDir: t.TempDir(),
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid timezone", func(t *testing.T) {
		_, err := New(
			context.Background(),
			Config{
				SkillsDir: t.TempDir(),
				TimeZone:  "Invalid/Zone",
			},
		)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}


func TestNewLoadsCheckedInSkills(t *testing.T) {
	// Production CLI uses the repository skills/ directory. Verify the real
	// shipped asset, not just the empty temp directory exercised above.
	dir := filepath.Join("..", "..", "skills")
	manifest := filepath.Join(dir, "tushare-data", "SKILL.md")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("required production skill %q is not checked in: %v", manifest, err)
	}
	if !strings.Contains(string(data), "name: tushare-data") {
		t.Fatalf("skill manifest %q has unexpected metadata", manifest)
	}
	rt, err := New(context.Background(), Config{
		SkillsDir: dir,
		TimeZone:  "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("real production skills could not be loaded: %v", err)
	}
	if len(rt.Handlers()) != 3 {
		t.Fatalf("loaded runtime handlers = %d, want 3", len(rt.Handlers()))
	}
}
