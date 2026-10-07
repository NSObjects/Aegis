package portfolio

type State string

const (
	StateNotProvided State = "NOT_PROVIDED"
	StateEmpty       State = "EMPTY"
	StatePositions   State = "POSITIONS"
)

type Position struct {
	Symbol   string  `json:"symbol"`
	Quantity float64 `json:"quantity"`
}

type Context struct {
	State     State      `json:"state"`
	AsOf      string     `json:"as_of,omitempty"`
	Positions []Position `json:"positions,omitempty"`
}
