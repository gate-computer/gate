package peer

type Logger interface {
	Printf(string, ...interface{})
}
