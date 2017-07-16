package run

import "path"

const (
	DefaultMaxProcs = 32767 - 1 // practical maximum (minus init process)
)

type Config struct {
	LibDir     string
	Container  string
	Executor   string
	Loader     string
	RuntimeMap string

	Uids [2]uint
	Gids [3]uint

	MaxProcs int
}

func (c *Config) libpath(name string) string {
	if path.IsAbs(name) {
		return name
	} else {
		return path.Join(c.LibDir, name)
	}
}

func (c *Config) container() string {
	name := c.Container
	if name == "" {
		name = "container"
	}
	return c.libpath(name)
}

func (c *Config) executor() string {
	name := c.Executor
	if name == "" {
		name = "executor"
	}
	return c.libpath(name)
}

func (c *Config) loader() string {
	name := c.Loader
	if name == "" {
		name = "loader"
	}
	return c.libpath(name)
}

func (c *Config) runtimeMap() string {
	name := c.RuntimeMap
	if name == "" {
		name = "runtime.map"
	}
	return c.libpath(name)
}

func (c *Config) maxProcs() int64 {
	if c.MaxProcs == 0 {
		return DefaultMaxProcs
	} else {
		return int64(c.MaxProcs)
	}
}
