package serverconfig

type Logger interface {
	Printf(string, ...interface{})
}
