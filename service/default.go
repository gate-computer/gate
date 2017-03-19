package service

var DefaultRegistry = new(Registry)

func Register(r *Registry, name string, version uint32, f func(evs chan<- []byte) Instance) {
	if r == nil {
		r = DefaultRegistry
	}
	r.Register(name, version, f)
}
