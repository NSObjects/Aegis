package debate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type retryScriptedModel struct {
	failures    int
	providerErr error
	output      string
	calls       int
}

func (m *retryScriptedModel) Generate(
	_ context.Context,
	_ []*schema.AgenticMessage,
	_ ...model.Option,
) (*schema.AgenticMessage, error) {
	m.calls++
	if m.calls <= m.failures {
		return nil, m.providerErr
	}
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: m.output}),
		},
	}, nil
}

func (m *retryScriptedModel) Stream(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	return nil, errors.New("stream should not be called")
}

func TestGenerateTurnRetriesTransientProviderFailure(t *testing.T) {
	originalDelay := debateProviderRetryDelay
	debateProviderRetryDelay = 0
	defer func() { debateProviderRetryDelay = originalDelay }()

	m := &retryScriptedModel{
		failures:    1,
		providerErr: errors.New("429 rate limit exceeded"),
		output:      validBullTurn,
	}
	bull := &BullResearcher{Model: m}
	turn, err := bull.GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err != nil {
		t.Fatalf("transient provider failure must be retried: %v", err)
	}
	if m.calls != 2 {
		t.Fatalf("calls = %d, want exactly one retry (2)", m.calls)
	}
	if turn.Arguments[0].ID != "bull-r1-1" {
		t.Fatalf("retried turn = %+v", turn)
	}
}

func TestGenerateTurnFailsAfterRepeatedProviderFailures(t *testing.T) {
	originalDelay := debateProviderRetryDelay
	debateProviderRetryDelay = 0
	defer func() { debateProviderRetryDelay = originalDelay }()

	m := &retryScriptedModel{
		failures:    2,
		providerErr: errors.New("connection reset by peer"),
		output:      validBullTurn,
	}
	_, err := (&BullResearcher{Model: m}).GenerateTurn(context.Background(), validArtifact(), &State{Symbol: "600519.SH"}, 1)
	if err == nil || !strings.Contains(err.Error(), "generate bull researcher turn") || m.calls != 2 {
		t.Fatalf("repeated provider failures must fail fast: calls=%d err=%v", m.calls, err)
	}
}

func TestGenerateWithProviderRetryDoesNotRetryCancelledContext(t *testing.T) {
	originalDelay := debateProviderRetryDelay
	debateProviderRetryDelay = time.Hour // would hang the test if retried
	defer func() { debateProviderRetryDelay = originalDelay }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &retryScriptedModel{
		failures:    1,
		providerErr: errors.New("late failure"),
		output:      validBullTurn,
	}
	_, err := generateWithProviderRetry(ctx, m, nil)
	if err == nil || !strings.Contains(err.Error(), "late failure") {
		t.Fatalf("cancelled context must surface the provider error: %v", err)
	}
	if m.calls != 1 {
		t.Fatalf("calls = %d, cancelled context must not retry", m.calls)
	}
}
