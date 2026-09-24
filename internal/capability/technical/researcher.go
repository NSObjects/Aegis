package technical

import "context"

type Request struct {
	Symbol string `json:"symbol" jsonschema:"required" jsonschema_description:"A-share symbol, for example 600519.SH"`
	AsOf   string `json:"as_of" jsonschema:"required" jsonschema_description:"Research date in YYYYMMDD format"`
}

type Researcher interface {
	Research(
		ctx context.Context,
		req Request,
	) (map[string]any, error)
}
