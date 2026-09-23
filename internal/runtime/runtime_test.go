package runtime

import (
	"context"
	"os"
	"path/filepath"
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
