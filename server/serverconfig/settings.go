package serverconfig

const (
	DefaultMemorySizeLimit = 16777216
	DefaultStackSize       = 65536
	DefaultPreforkProcs    = 1
)

type Settings struct {
	MemorySizeLimit int
	StackSize       int
	PreforkProcs    int
}
