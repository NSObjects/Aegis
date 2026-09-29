package event

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPDoer is the minimal transport dependency shared by HTTP-backed
// retrievers. It appeared only after the second real retriever needed the same
// seam, keeping the abstraction evidence-driven rather than speculative.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func requireHTTP2xx(
	resp *http.Response,
	context string,
) error {
	if resp.StatusCode >= 200 &&
		resp.StatusCode < 300 {
		return nil
	}

	message := ""
	if resp.Body != nil {
		body, _ := io.ReadAll(
			io.LimitReader(resp.Body, 512),
		)
		message = strings.TrimSpace(string(body))
	}

	if message == "" {
		return fmt.Errorf(
			"%s returned HTTP %d",
			context,
			resp.StatusCode,
		)
	}
	return fmt.Errorf(
		"%s returned HTTP %d: %s",
		context,
		resp.StatusCode,
		message,
	)
}
