package runtime

import (
	"Aegis/internal/middleware"
	"time"

	"github.com/cloudwego/eino/adk"
)

// NewContextMiddleware is kept temporarily for callers that have not yet
// migrated to Runtime. New code should use middleware.NewContext directly
// or obtain the complete handler set from Runtime.Handlers.
func NewContextMiddleware[M adk.MessageType](
	location *time.Location,
) *middleware.Context[M] {
	return middleware.NewContext[M](location)
}
