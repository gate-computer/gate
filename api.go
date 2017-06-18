package gate

const (
	ResultHTTPHeaderName = "X-Gate-Result"
)

type Result struct {
	Exit  int    `json:"exit,omitempty"`
	Trap  string `json:"trap,omitempty"`
	Error string `json:"error,omitempty"`
}
