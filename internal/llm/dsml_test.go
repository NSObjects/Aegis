package llm

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestParseDSMLTextMangledExecute(t *testing.T) {
	raw := `[System: Empty message content sanitised to satisfy protocol]

<｜｜DSML｜｜ calls>
<｜｜DSML｜｜ invoke name="execute">
<｜｜DSML｜｜ parameter name="command" string="true">python3 q8.py 2>&1 | grep -v warning</｜｜DSML｜｜ parameter>
</｜｜DSML｜｜ invoke>
</｜｜DSML｜｜ calls>`

	seq := 0
	blocks, found, err := parseDSMLText(
		raw,
		func() string {
			seq++
			return "call-1"
		},
	)
	if err != nil {
		t.Fatalf("parseDSMLText() error = %v", err)
	}

	if !found {
		t.Fatal("expected DSML call")
	}

	if len(blocks) != 1 {
		t.Fatalf(
			"blocks = %d, want 1",
			len(blocks),
		)
	}

	call := blocks[0].FunctionToolCall
	if call == nil {
		t.Fatal("expected function tool call")
	}

	if call.Name != "execute" {
		t.Fatalf(
			"tool name = %q, want execute",
			call.Name,
		)
	}

	var args map[string]any
	if err := json.Unmarshal(
		[]byte(call.Arguments),
		&args,
	); err != nil {
		t.Fatalf(
			"decode arguments: %v",
			err,
		)
	}

	if args["command"] !=
		"python3 q8.py 2>&1 | grep -v warning" {
		t.Fatalf(
			"unexpected command: %#v",
			args["command"],
		)
	}
}

func TestParseDSMLTextCanonicalValues(t *testing.T) {
	raw := `checking
<｜DSML｜tool_calls>
<｜DSML｜invoke name="tool">
<｜DSML｜parameter name="symbol" string="true">600519.SH</｜DSML｜parameter>
<｜DSML｜parameter name="limit" string="false">20</｜DSML｜parameter>
<｜DSML｜parameter name="flags" string="false">["a","b"]</｜DSML｜parameter>
</｜DSML｜invoke>
</｜DSML｜tool_calls>`

	blocks, found, err := parseDSMLText(
		raw,
		func() string {
			return "call-1"
		},
	)
	if err != nil {
		t.Fatalf("parseDSMLText() error = %v", err)
	}

	if !found {
		t.Fatal("expected DSML call")
	}

	if len(blocks) != 2 {
		t.Fatalf(
			"blocks = %d, want 2",
			len(blocks),
		)
	}

	if blocks[0].Type !=
		schema.ContentBlockTypeAssistantGenText {
		t.Fatalf(
			"first block type = %s",
			blocks[0].Type,
		)
	}

	call := blocks[1].FunctionToolCall
	if call == nil {
		t.Fatal("expected function tool call")
	}

	var args map[string]any
	if err := json.Unmarshal(
		[]byte(call.Arguments),
		&args,
	); err != nil {
		t.Fatal(err)
	}

	if args["symbol"] != "600519.SH" {
		t.Fatalf(
			"symbol = %#v",
			args["symbol"],
		)
	}

	if args["limit"] != float64(20) {
		t.Fatalf(
			"limit = %#v",
			args["limit"],
		)
	}
}

func TestParseDSMLTextPlainText(t *testing.T) {
	blocks, found, err := parseDSMLText(
		`{"action":"WATCH"}`,
		func() string {
			return "unused"
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if found {
		t.Fatal("plain text must not be treated as DSML")
	}

	if blocks != nil {
		t.Fatalf(
			"blocks = %#v, want nil",
			blocks,
		)
	}
}

func TestParseDSMLTextMalformedParameter(t *testing.T) {
	raw := `<｜DSML｜tool_calls>
<｜DSML｜invoke name="tool">
<｜DSML｜parameter name="limit" string="false">not-json</｜DSML｜parameter>
</｜DSML｜invoke>
</｜DSML｜tool_calls>`

	_, found, err := parseDSMLText(
		raw,
		func() string {
			return "call-1"
		},
	)

	if !found {
		t.Fatal("expected DSML call")
	}

	if err == nil {
		t.Fatal("expected error")
	}
}
