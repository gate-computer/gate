package webapi

const (
	HeaderProgramId     = "X-Gate-Program-Id"     // opaque
	HeaderProgramSHA512 = "X-Gate-Program-Sha512" // hexadecimal
	HeaderInstanceId    = "X-Gate-Instance-Id"    // opaque
	HeaderExitStatus    = "X-Gate-Exit-Status"    // non-negative integer
	HeaderTrap          = "X-Gate-Trap"           // human-readable JSON
	HeaderTrapId        = "X-Gate-Trap-Id"        // positive integer
	HeaderError         = "X-Gate-Error"          // human-readable JSON
	HeaderErrorId       = "X-Gate-Error-Id"       // positive integer
)

type Run struct {
	ProgramId     string `json:"program_id,omitempty"`
	ProgramSHA512 string `json:"program_sha512,omitempty"`
}

type Running struct {
	InstanceId string `json:"instance_id"`
	ProgramId  string `json:"program_id,omitempty"`
}

type Communicate struct {
	InstanceId string `json:"instance_id"`
}

type Communicating struct {
}

type Result struct {
	ExitStatus *int   `json:"exit_status,omitempty"`
	Trap       string `json:"trap,omitempty"`
	TrapId     int    `json:"trap_id,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorId    int    `json:"error_id,omitempty"`
}
