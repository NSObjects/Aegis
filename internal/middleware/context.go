package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
)

type Context[M adk.MessageType] struct {
	*adk.TypedBaseChatModelAgentMiddleware[M]

	location *time.Location
	now      func() time.Time
}

func NewContext[M adk.MessageType](location *time.Location) *Context[M] {
	if location == nil {
		location = time.Local
	}

	return &Context[M]{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[M]{},
		location:                          location,
		now:                               time.Now,
	}
}

func (m *Context[M]) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (
	context.Context,
	*adk.ChatModelAgentContext,
	error,
) {
	if runCtx == nil {
		return ctx, nil, nil
	}

	now := m.now().In(m.location)

	runtimeContext := fmt.Sprintf(`
# Runtime Context

The following values are runtime facts provided by the Aegis Harness.

- current_time: %s
- timezone: %s

Rules:

- Treat current_time as the actual runtime clock, not as the target time of the user's research request.
- Determine the target research time from the user's language and intent.
- Relative expressions such as "当前", "今天", "最近", "昨天" may use current_time as their reference.
- Explicit historical dates or ranges must remain historical and must not be replaced by current_time.
- Domain-specific time decisions, such as trading days, market sessions, data completeness, and latest available market data, must be determined using the appropriate Skill or tools.
`,
		now.Format(time.RFC3339),
		m.location.String(),
	)

	next := *runCtx
	next.Instruction += runtimeContext

	return ctx, &next, nil
}
