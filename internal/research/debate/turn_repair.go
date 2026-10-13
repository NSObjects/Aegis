package debate

import (
	"Aegis/internal/research/synthesis"
	"Aegis/internal/researchlang"
	"context"
	"time"
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

// debateProviderRetryDelay backs off the single retry after a transient
// provider failure (rate limit, dropped connection). Cancelled contexts and
// repeated provider failures are never retried twice.
var debateProviderRetryDelay = 1500 * time.Millisecond

// generateWithProviderRetry retries one transient provider failure. Format
// and contract failures are not retried here; they flow through the regular
// turn-correction attempts in generateValidatedTurn.
func generateWithProviderRetry(
	ctx context.Context,
	chatModel model.BaseModel[*schema.AgenticMessage],
	messages []*schema.AgenticMessage,
) (*schema.AgenticMessage, error) {
	response, err := chatModel.Generate(ctx, messages)
	if err == nil {
		return response, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, err
	case <-time.After(debateProviderRetryDelay):
	}
	return chatModel.Generate(ctx, messages)
}

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
		response, err := generateWithProviderRetry(ctx, chatModel, messages)
		if err != nil {
			// Provider failures keep failing fast after one backed-off
			// retry; malformed turns below get a corrective regeneration.
			return Turn{}, fmt.Errorf("generate %s researcher turn: %w", strings.ToLower(string(expectedSide)), err)
		}
		turn, validationErr := parseValidatedTurn(response, artifact, state, round, expectedSide)
		if validationErr == nil {
            validationErr=checkTurnLanguage(ctx,turn)
            if validationErr==nil{return turn,nil}
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
				"when an opposing argument exists in debate_state, rebuttals MUST target at least one real prior opposing argument ID. "+
				"Other rebuttals may refer only to argument IDs already in the supplied debate_state. "+
				"Do not invent facts, references, events or prior argument IDs. "+
				"Return only the complete corrected JSON object. %s",
            validationErr, expectedSide, round,researchlang.Correction,
		)))
	}
	return Turn{}, errors.New("debate turn validation exhausted")
}

func checkTurnLanguage(ctx context.Context,turn Turn) error {
    fields:=[]researchlang.Field{{Name:"debate.thesis",Text:turn.Thesis}}
    for _,a:=range turn.Arguments{fields=append(fields,researchlang.Field{Name:"debate.arguments.claim",Text:a.Claim})}
    for _,r:=range turn.Rebuttals{fields=append(fields,researchlang.Field{Name:"debate.rebuttals.claim",Text:r.Claim})}
    return researchlang.Check(ctx,fields...)
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
