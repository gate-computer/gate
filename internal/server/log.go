package server

type Logger interface {
	Printf(string, ...interface{})
}
