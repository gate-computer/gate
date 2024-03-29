# Copyright (c) 2017 Timo Savola. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

from collections import defaultdict
from ctypes import CDLL, c_ulong
from os import sysconf
from resource import RLIMIT_STACK, setrlimit
from struct import unpack
from subprocess import PIPE, Popen
from sys import argv

ADDR_NO_RANDOMIZE = c_ulong(0x0040000)

GATE_LOADER_STACK_SIZE = 12288  # Must match the definition in runtime/include/runtime.hpp

page = sysconf("SC_PAGESIZE")
assert (GATE_LOADER_STACK_SIZE % page) == 0

libc = CDLL("libc.so.6")


def preexec():
    ret = libc.personality(ADDR_NO_RANDOMIZE)
    if ret < 0:
        raise Exception(ret)

    setrlimit(RLIMIT_STACK, (GATE_LOADER_STACK_SIZE, GATE_LOADER_STACK_SIZE))


min_alloc = None
max_alloc = None
min_usage = None
max_usage = None
instances = defaultdict(int)

for _ in range(100):
    proc = Popen(argv[1:], stdout=PIPE, preexec_fn=preexec, env={})
    output = proc.stdout.read()
    code = proc.wait()
    if code != 0:
        raise Exception(code)

    assert len(output) == 8 * 5, len(output)

    init_addr, low_addr, hand_addr, high_addr, _ = unpack(b"QQQQQ", output)
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
print("signal handler stack usage (approx) = %d" % hand_addr)

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
