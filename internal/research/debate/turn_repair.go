package debate

import (
	"Aegis/internal/research/synthesis"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// A failed model-generated debate turn may be corrected once. The model must
// supply a complete grounded turn on retry; the runtime never manufactures
// argument IDs, references, claims or rebuttal targets.
const maxDebateTurnAttempts = 2

func generateValidatedTurn(
	ctx context.Context,
	chatModel model.BaseModel[*schema.AgenticMessage],
	systemInstruction string,
	payload string,
	artifact *synthesis.Artifact,
	state *State,
	round int,
	expectedSide Side,
) (Turn, error) {
	if chatModel == nil {
		return Turn{}, errors.New("debate researcher model is required")
	}

	messages := []*schema.AgenticMessage{
		schema.SystemAgenticMessage(systemInstruction),
		schema.UserAgenticMessage(payload),
	}
	for attempt := 1; attempt <= maxDebateTurnAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Turn{}, err
		}
		response, err := chatModel.Generate(ctx, messages)
		if err != nil {
			// Provider errors are not malformed debate turns. Do not multiply
			// network requests or hide upstream failures behind a retry.
			return Turn{}, fmt.Errorf("generate %s researcher turn: %w", strings.ToLower(string(expectedSide)), err)
		}
		turn, validationErr := parseValidatedTurn(response, artifact, state, round, expectedSide)
		if validationErr == nil {
			return turn, nil
		}
		if attempt == maxDebateTurnAttempts {
			return Turn{}, fmt.Errorf("invalid %s researcher turn after %d attempts: %w",
				strings.ToLower(string(expectedSide)), attempt, validationErr)
		}
		// Preserve the original complete artifact and debate state. Only
		// feedback about the invalid structured output is added. Ask the
		// model to generate a new, independently validated JSON response.
		messages = append([]*schema.AgenticMessage(nil), messages[:2]...)
		messages = append(messages, schema.UserAgenticMessage(fmt.Sprintf(
			"Your previous debate turn failed the required domain contract: %s. "+
				"Generate a complete NEW JSON turn for side %s, round %d, "+
				"using only the research_artifact and debate_state already supplied. "+
				"Every argument MUST have an explicit, unique, nonempty id and a grounded claim and references; "+
				"rebuttals may refer only to argument IDs already in the supplied debate_state. "+
				"Do not invent facts, references, events or prior argument IDs. "+
				"Return only the complete corrected JSON object.",
			validationErr, expectedSide, round,
		)))
	}
	return Turn{}, errors.New("debate turn validation exhausted")
}

func parseValidatedTurn(
	response *schema.AgenticMessage,
	artifact *synthesis.Artifact,
	state *State,
	round int,
	expectedSide Side,
) (Turn, error) {
	raw := strings.TrimSpace(debateAssistantText(response))
	if raw == "" {
		return Turn{}, errors.New("researcher returned empty output")
	}
	turn, err := decodeTurn(raw)
	if err != nil {
		return Turn{}, err
	}
	if turn.Side != expectedSide {
		return Turn{}, fmt.Errorf("%s researcher returned side %q",
			strings.ToLower(string(expectedSide)), turn.Side)
	}
	if turn.Round != round {
		return Turn{}, fmt.Errorf("%s researcher returned round %d, want %d",
			strings.ToLower(string(expectedSide)), turn.Round, round)
	}
	// Validate against prior turns, not against an invented or repaired
	// state. This enforces argument-ID uniqueness and grounded references.
	candidate := &State{
		Symbol: state.Symbol,
		Turns: append(append([]Turn(nil), state.Turns...), turn),
	}
	if err := ValidateState(candidate, artifact); err != nil {
        return Turn{}, fmt.Errorf("invalid %s researcher turn: %w", strings.ToLower(string(expectedSide)), err)
    }
    opposing := SideBull
    if expectedSide == SideBull { opposing = SideBear }
    if err := requireOpposingEngagement(turn, state, opposing); err != nil {
        return Turn{}, fmt.Errorf("invalid %s researcher turn: %w", strings.ToLower(string(expectedSide)), err)
    }
    return turn, nil
}
