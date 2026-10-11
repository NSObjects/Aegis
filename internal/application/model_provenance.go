package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"Aegis/internal/config"
)

// ModelRoleBinding names a model configuration used to construct one role.
// Model is the configured API model ID, NOT a verified provider revision.
// Never add API keys, endpoints, headers or raw provider parameters here.
type ModelRoleBinding struct {
	Role string `json:"role"`
	Profile string `json:"profile"`
	Provider string `json:"provider"`
	Model string `json:"model"`
}

// RunModelProvenance is recorded when the live graph was successfully built,
// before the paid research workflow runs. It is absent on legacy runs, and on
// failures before graph construction. Fingerprint hashes ONLY these bindings,
// not config.yaml (which can contain sensitive URLs or future private fields).
type RunModelProvenance struct {
	SchemaVersion int `json:"schema_version"`
	ConfigFingerprint string `json:"config_fingerprint"`
	Bindings []ModelRoleBinding `json:"bindings"`
}

type modelRoleSelection struct { Role, Profile string }

func buildModelProvenance(cfg *config.Config, roles []modelRoleSelection) (*RunModelProvenance, error) {
	if cfg == nil || len(roles) == 0 {
		return nil, errors.New("model provenance requires configured roles")
	}
	bindings := make([]ModelRoleBinding, 0, len(roles))
	for _, role := range roles {
		model, ok := cfg.Models[role.Profile]
		if !ok {
			return nil, fmt.Errorf("missing model profile for role %q", role.Role)
		}
		if role.Role == "" || role.Profile == "" || model.Provider == "" || model.Model == "" {
			return nil, fmt.Errorf("incomplete model role binding %q", role.Role)
		}
		bindings = append(bindings, ModelRoleBinding{
			Role: role.Role, Profile: role.Profile,
			Provider: string(model.Provider), Model: model.Model,
		})
	}
	result := &RunModelProvenance{SchemaVersion: 1, Bindings: bindings}
	digest, err := modelBindingsDigest(bindings)
	if err != nil { return nil, err }
	result.ConfigFingerprint = digest
	if err := validateRunModelProvenance(result); err != nil { return nil, err }
	return result, nil
}

func modelBindingsDigest(bindings []ModelRoleBinding) (string, error) {
	data, err := json.Marshal(bindings)
	if err != nil { return "", fmt.Errorf("encode model bindings: %w", err) }
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func validateRunModelProvenance(meta *RunModelProvenance) error {
	if meta == nil { return nil } // older records and early initialization failures
	if meta.SchemaVersion != 1 || len(meta.Bindings) == 0 || len(meta.Bindings) > 16 {
		return errors.New("invalid model provenance version or binding count")
	}
	seen := make(map[string]bool, len(meta.Bindings))
	for _, binding := range meta.Bindings {
		if len(binding.Role) == 0 || len(binding.Role) > 64 ||
			len(binding.Profile) == 0 || len(binding.Profile) > 128 ||
			len(binding.Provider) == 0 || len(binding.Provider) > 64 ||
			len(binding.Model) == 0 || len(binding.Model) > 128 ||
			seen[binding.Role] {
			return errors.New("invalid or duplicate model provenance role")
		}
		for _, val := range []string{binding.Role, binding.Profile, binding.Provider, binding.Model} {
			if strings.TrimSpace(val) != val || strings.ContainsAny(val, "\r\n\x00") {
				return errors.New("invalid model provenance value")
			}
		}
		seen[binding.Role] = true
	}
	expected, err := modelBindingsDigest(meta.Bindings)
	if err != nil || meta.ConfigFingerprint != expected {
		return errors.New("model provenance fingerprint mismatch")
	}
	return nil
}

type modelProvenanceSinkKey struct{}
type modelProvenanceSink func(*RunModelProvenance) error

func withModelProvenanceSink(ctx context.Context, sink modelProvenanceSink) context.Context {
	return context.WithValue(ctx, modelProvenanceSinkKey{}, sink)
}

// publishModelProvenance is called by the shared live graph factory only when
// all model components were constructed. Outside RunManager (CLI) it is a no-op.
func publishModelProvenance(ctx context.Context, meta *RunModelProvenance) error {
	if sink, ok := ctx.Value(modelProvenanceSinkKey{}).(modelProvenanceSink); ok {
		return sink(meta)
	}
	return nil
}
