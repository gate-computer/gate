package service

var DefaultRegistry = new(Registry)

func Register(r *Registry, name string, version int32, f Factory) {
	if r == nil {
		r = DefaultRegistry
	}
	r.Register(name, version, f)
}

func RegisterFunc(r *Registry, name string, version int32, f func() Instance) {
	Register(r, name, version, FactoryFunc(f))
}
