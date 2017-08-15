# -*- encoding: utf-8 -*-

import collections
import ctypes
import os
import resource
import struct
import subprocess

ADDR_NO_RANDOMIZE = ctypes.c_ulong(0x0040000)

GATE_LOADER_STACK_SIZE = 12288  # must match the definition in defs.h

page = os.sysconf("SC_PAGESIZE")
assert (GATE_LOADER_STACK_SIZE % page) == 0

libc = ctypes.CDLL("libc.so.6")


def preexec():
    ret = libc.personality(ADDR_NO_RANDOMIZE)
    if ret < 0:
        raise Exception(ret)

    resource.setrlimit(resource.RLIMIT_STACK, (GATE_LOADER_STACK_SIZE, GATE_LOADER_STACK_SIZE))


argv = []
envp = {"/": "XXXXXXXXXXXXXXXX/self/fd/X"}  # matches GATE_FD_PATH_LEN

min_alloc = None
max_alloc = None
min_usage = None
max_usage = None
instances = collections.defaultdict(int)

for _ in range(10000):
    proc = subprocess.Popen(argv, executable="./stack", stdout=subprocess.PIPE, preexec_fn=preexec, env=envp)
    output = proc.stdout.read()
    code = proc.wait()
    if code != 0:
        raise Exception(code)

    assert len(output) == 8 * 3, len(output)

    init_addr, low_addr, high_addr = struct.unpack(b"QQQ", output)
    low_addr += 8  # skip over the faulting address
    alloc = high_addr - low_addr
    usage = high_addr - init_addr

    if min_alloc is None:
        min_alloc = alloc
        max_alloc = alloc
        min_usage = usage
        max_usage = usage
    else:
        min_alloc = min(min_alloc, alloc)
        max_alloc = max(max_alloc, alloc)
        min_usage = min(min_usage, usage)
        max_usage = max(max_usage, usage)

    instances[(alloc, usage)] += 1

print("min stack alloc limit   = %d" % min_alloc)
print("max stack alloc limit   = %d" % max_alloc)
print("min initial stack usage = %d" % min_usage)
print("max initial stack usage = %d" % max_usage)

if 0:
    print("")
    print("  ALLOC USAGE NUM")

    for (alloc, usage), num in sorted(instances.items()):
        print(("  %5d %5d %3d %s٥" % (alloc, usage, num, "٠" * (num - 1))))

    print("")

assert min_alloc == GATE_LOADER_STACK_SIZE, "min stack alloc limit == %d" % GATE_LOADER_STACK_SIZE
assert max_alloc == GATE_LOADER_STACK_SIZE, "max stack alloc limit == %d" % GATE_LOADER_STACK_SIZE
assert min_usage > 0, "min initial stack usage > 0"
assert max_usage < page, "max initial stack usage < %d" % page
