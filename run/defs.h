// some of these are also defined in defs.go, run.js and work.js

#define GATE_RODATA_ADDR 0x10000

#define GATE_LOADER_STACK_PAGES 3 // minimum workable value, determined on Linux 4.2

#define GATE_BLOCK_FD    0
#define GATE_BLOCK_PATH  "/proc/self/fd/0"
#define GATE_OUTPUT_FD   1
#define GATE_DEBUG_FD    2
#define GATE_MAPS_FD     3
#define GATE_NONBLOCK_FD 3
#define GATE_LOADER_FD   4
#define GATE_WAKEUP_FD   5

#define GATE_SIGNAL_STACK_RESERVE   0x600 // TODO
#define GATE_SIGNAL_STACK_R9_OFFSET 56

#define GATE_ABI_VERSION     0
#define GATE_MAX_PACKET_SIZE 0x10000
