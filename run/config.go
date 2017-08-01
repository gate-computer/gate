package run

const (
	DefaultMaxProcs = 32767 - 1 // practical maximum (minus init process)

	DefaultCgroupTitle = "gate-executor"
)

type Config struct {
	LibDir string

	Uids [2]uint
	Gids [3]uint

	MaxProcs int

	// These have no effect if container was compiled without cgroup support.
	CgroupParent string
	CgroupTitle  string
}

func (c *Config) maxProcs() int64 {
	if c.MaxProcs == 0 {
		return DefaultMaxProcs
	} else {
		return int64(c.MaxProcs)
	}
}

func (c *Config) cgroupTitle() (s string) {
	s = c.CgroupTitle
	if s == "" {
		s = DefaultCgroupTitle
	}
	return
}
