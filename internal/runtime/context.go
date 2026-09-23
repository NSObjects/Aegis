package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
)

type contextMiddleware[M adk.MessageType] struct {
	*adk.TypedBaseChatModelAgentMiddleware[M]

	location *time.Location
	now      func() time.Time
}

func newContextMiddleware[M adk.MessageType](location *time.Location) *contextMiddleware[M] {
	if location == nil {
		location = time.Local
	}

	return &contextMiddleware[M]{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[M]{},

		location: location,
		now:      time.Now,
	}
}

func (m *contextMiddleware[M]) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil {
		return ctx, nil, nil
	}

	now := m.now().In(m.location)

	runtimeContext := fmt.Sprintf(
		`# Runtime Context

The following values are runtime facts provided by the Aegis Harness.

- current_time: %s
- timezone: %s

Rules:

- Treat current_time as an environment fact, not as the target time of the user's request.
- Determine the target research time from the user's language and intent.
- Relative expressions such as "当前", "今天", "最近", and "昨天" may use current_time as their reference.
- Explicit historical dates and ranges must remain historical and must not be replaced by current_time.
- Trading days, market sessions, data completeness, latest available data, and other domain-specific time decisions must be determined using the appropriate Skill or tools.`,
		now.Format(time.RFC3339),
		m.location.String(),
	)

	next := *runCtx

	if next.Instruction == "" {
		next.Instruction = runtimeContext
	} else {
		next.Instruction += "\n\n" + runtimeContext
	}

	return ctx, &next, nil
}
