package llm

import (
	"Aegis/internal/config"
	"context"
	"strings"
	"testing"
)

func TestAPIKey(t *testing.T) {
	t.Run("reads configured environment variable", func(t *testing.T) {
		t.Setenv("TEST_MODEL_API_KEY", "test-key")

		got, err := apiKey(config.ModelConfig{
			APIKeyEnv: "TEST_MODEL_API_KEY",
		})
		if err != nil {
			t.Fatalf("apiKey() error = %v", err)
		}

		if got != "test-key" {
			t.Fatalf("apiKey() = %q, want %q", got, "test-key")
		}
	})

	t.Run("requires environment variable name", func(t *testing.T) {
		_, err := apiKey(config.ModelConfig{})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("requires environment variable value", func(t *testing.T) {
		t.Setenv("TEST_EMPTY_MODEL_API_KEY", "")

		_, err := apiKey(config.ModelConfig{
			APIKeyEnv: "TEST_EMPTY_MODEL_API_KEY",
		})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "TEST_EMPTY_MODEL_API_KEY") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestManagerGetMissingProfile(t *testing.T) {
	manager := NewManager(nil)

	_, err := manager.Get(
		context.Background(),
		"missing",
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewUnsupportedProvider(t *testing.T) {
	_, err := New(
		context.Background(),
		config.ModelConfig{
			Provider: "unknown",
			Model:    "test",
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
}
