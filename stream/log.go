package stream

// Logger
type Logger interface {
	Printf(format string, v ...interface{})
}

var (
	DebugLog Logger
)

func logf(format string, v ...interface{}) {
	if DebugLog != nil {
		DebugLog.Printf(format, v...)
	}
}
