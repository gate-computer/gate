package gate

type Program struct {
	Id     string `json:"id,omitempty"`
	SHA512 string `json:"sha512,omitempty"`
}

type Instance struct {
	Id string `json:"id,omitempty"`
}

type Result struct {
	Exit  int    `json:"exit"`
	Trap  string `json:"trap,omitempty"`
	Error string `json:"error,omitempty"`
}

// Requests

type Load struct {
	Program Program `json:"program"`
}

type Run struct {
	Program Program `json:"program"`
}

type Origin struct {
	Instance Instance `json:"instance"`
}

type Wait struct {
	Instance Instance `json:"instance"`
}

// Responses

type Loaded struct {
	Program *Program `json:"program,omitempty"`
}

type Running struct {
	Instance Instance `json:"instance"`
	Program  *Program `json:"program,omitempty"`
}

type Finished struct {
	Result Result `json:"result"`
}
