package service

var Defaults = new(Registry)

func Register(r *Registry, name string, version int32, f Factory) {
	if r == nil {
		r = Defaults
	}
	r.Register(name, version, f)
}

func RegisterFunc(r *Registry, name string, version int32, f func() Instance) {
	Register(r, name, version, FactoryFunc(f))
}

func Clone(r *Registry) *Registry {
	if r == nil {
		r = Defaults
	}
	return r.Clone()
}
