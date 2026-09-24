package middleware

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestContextBeforeAgent(t *testing.T) {
	location := time.FixedZone(
		"Asia/Shanghai",
		8*60*60,
	)

	middleware := NewContext[*schema.AgenticMessage](
		location,
	)

	middleware.now = func() time.Time {
		return time.Date(
			2026,
			9,
			23,
			14,
			10,
			0,
			0,
			location,
		)
	}

	original := &adk.ChatModelAgentContext{
		Instruction: "market instruction",
	}

	_, got, err := middleware.BeforeAgent(
		context.Background(),
		original,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got == nil {
		t.Fatal("runtime context is nil")
	}

	if !strings.Contains(
		got.Instruction,
		"2026-09-23T14:10:00+08:00",
	) {
		t.Fatalf(
			"current_time missing from instruction:\n%s",
			got.Instruction,
		)
	}

	if !strings.Contains(
		got.Instruction,
		"Asia/Shanghai",
	) {
		t.Fatalf(
			"timezone missing from instruction:\n%s",
			got.Instruction,
		)
	}

	if original.Instruction != "market instruction" {
		t.Fatalf(
			"original run context was mutated: %s",
			original.Instruction,
		)
	}
}

func TestContextBeforeAgentNilContext(t *testing.T) {
	middleware := NewContext[*schema.AgenticMessage](
		time.UTC,
	)

	_, got, err := middleware.BeforeAgent(
		context.Background(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != nil {
		t.Fatal("expected nil run context")
	}
}
