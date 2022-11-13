// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#define __wasi__

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <gate.h>
#include <wasi/api.h>

#define STRARG(s) s, __builtin_strlen(s)

namespace {

size_t bytestrlen(uint8_t const* s)
{
	size_t n = 0;
	while (*s++)
		n++;
	return n;
}

} // namespace

#define TEST(name) \
	namespace test { \
		static __attribute__((always_inline)) void name(); \
	} \
	extern "C" void test_##name() \
	{ \
		test::name(); \
		gate_debug("PASS\n"); \
	} \
	void test::name()

#define TEST_TRAP(name) \
	namespace testtrap { \
		static __attribute__((always_inline)) void name(); \
	} \
	extern "C" void testtrap_##name() \
	{ \
		testtrap::name(); \
		gate_exit(1); \
	} \
	void testtrap::name()

#define ASSERT(expr) \
	do { \
		if (!(expr)) { \
			gate_debug(__FILE__ ":", __LINE__, ": " #expr "\n"); \
			gate_exit(1); \
		} \
	} while (0)

TEST(args)
{
	size_t count = 123;
	size_t bufsize = 456;
	ASSERT(__wasi_args_sizes_get(&count, &bufsize) == 0);
	ASSERT(count == 0);
	ASSERT(bufsize == 0);

	uint8_t dummy = 0;
	uint8_t* argv[1] = {&dummy};
	uint8_t argbuf[1] = {78};
	ASSERT(__wasi_args_get(argv, argbuf) == 0);
	ASSERT(argv[0] == &dummy);
	ASSERT(argbuf[0] == 78);
}

TEST(clock_res)
{
	for (__wasi_clockid_t id = __WASI_CLOCKID_REALTIME; id <= __WASI_CLOCKID_THREAD_CPUTIME_ID; id++) {
		__wasi_timestamp_t res = 123456789;
		ASSERT(__wasi_clock_res_get(id, &res) == 0);
		ASSERT(res == 1024);
	}

	__wasi_timestamp_t res = 0;
	ASSERT(__wasi_clock_res_get(__WASI_CLOCKID_THREAD_CPUTIME_ID + 1, &res) != 0);
}

TEST(clock_time)
{
	__wasi_timestamp_t realtime = 0;
	__wasi_timestamp_t monotonic = 0;
	ASSERT(__wasi_clock_time_get(__WASI_CLOCKID_REALTIME, 1, &realtime) == 0);
	ASSERT(__wasi_clock_time_get(__WASI_CLOCKID_MONOTONIC, 1, &monotonic) == 0);
	ASSERT(realtime > 0);
	ASSERT(monotonic > 0);

	__wasi_timestamp_t t = 0;
	ASSERT(__wasi_clock_time_get(__WASI_CLOCKID_THREAD_CPUTIME_ID + 1, 1, &t) != 0);
}

TEST_TRAP(clock_time_process)
{
	__wasi_timestamp_t t;
	(void) __wasi_clock_time_get(__WASI_CLOCKID_PROCESS_CPUTIME_ID, 1, &t);
}

TEST_TRAP(clock_time_thread)
{
	__wasi_timestamp_t t;
	(void) __wasi_clock_time_get(__WASI_CLOCKID_THREAD_CPUTIME_ID, 1, &t);
}

TEST(environ)
{
	size_t count = 123;
	size_t bufsize = 0;
	ASSERT(__wasi_environ_sizes_get(&count, &bufsize) == 0);
	ASSERT(count == 3);
	ASSERT(bufsize > 0 && bufsize < 1000);

	uint8_t* envv[3] = {nullptr, nullptr, nullptr};
	uint8_t envbuf[bufsize];
	ASSERT(__wasi_environ_get(envv, envbuf) == 0);
	for (int i = 0; i < 3; i++) {
		ASSERT((uintptr_t) envv[i] >= (uintptr_t) envbuf);
		ASSERT((uintptr_t) envv[i] + bytestrlen(envv[i]) < (uintptr_t) envbuf + bufsize);
	}
}

TEST(fd)
{
	ASSERT(__GATE_FD() == 4);
}

TEST(fd_advise)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_advise(fd, 0, 4096, __WASI_ADVICE_RANDOM) != 0);
}

TEST(fd_allocate)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_allocate(fd, 0, 8192) != 0);
}

TEST(fd_close)
{
	ASSERT(__wasi_fd_close(3) != 0);
	ASSERT(__wasi_fd_close(5) != 0);
}

TEST_TRAP(fd_close_stdin)
{
	(void) __wasi_fd_close(0);
}

TEST_TRAP(fd_close_stdout)
{
	(void) __wasi_fd_close(1);
}

TEST_TRAP(fd_close_stderr)
{
	(void) __wasi_fd_close(2);
}

TEST_TRAP(fd_close_gate)
{
	(void) __wasi_fd_close(__GATE_FD());
}

TEST(datasync)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_datasync(fd) != 0);
}

TEST(fd_fdstat_get)
{
	__wasi_fdstat_t stdin;
	__wasi_fdstat_t stdout;
	__wasi_fdstat_t stderr;
	__wasi_fdstat_t gate;
	__wasi_fdstat_t dummy;

	ASSERT(__wasi_fd_fdstat_get(0, &stdin) == 0);
	ASSERT(__wasi_fd_fdstat_get(1, &stdout) == 0);
	ASSERT(__wasi_fd_fdstat_get(2, &stderr) == 0);
	ASSERT(__wasi_fd_fdstat_get(3, &dummy) != 0);
	ASSERT(__wasi_fd_fdstat_get(__GATE_FD(), &gate) == 0);
	ASSERT(__wasi_fd_fdstat_get(5, &dummy) != 0);

	ASSERT(stdin.fs_flags == 0);
	ASSERT(stdout.fs_flags == 0);
	ASSERT(stderr.fs_flags == 0);
	ASSERT(gate.fs_flags == __WASI_FDFLAGS_NONBLOCK);

	ASSERT(stdin.fs_rights_base == 0);
	ASSERT(stdout.fs_rights_base == __WASI_RIGHTS_FD_WRITE);
	ASSERT(stderr.fs_rights_base == __WASI_RIGHTS_FD_WRITE);
	ASSERT(gate.fs_rights_base == (__WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE));

	ASSERT(stdin.fs_rights_inheriting == 0);
	ASSERT(stdout.fs_rights_inheriting == 0);
	ASSERT(stderr.fs_rights_inheriting == 0);
	ASSERT(gate.fs_rights_inheriting == 0);
}

TEST(fd_fdstat_set_flags)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_fdstat_set_flags(fd, __WASI_FDFLAGS_DSYNC) != 0);
}

TEST(fd_fdstat_set_rights)
{
	// stdin
	ASSERT(__wasi_fd_fdstat_set_rights(0, 0, 0) == 0);
	ASSERT(__wasi_fd_fdstat_set_rights(0, 0, __WASI_RIGHTS_FD_READ) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(0, __WASI_RIGHTS_FD_READ, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(0, __WASI_RIGHTS_FD_ALLOCATE, 0) != 0);

	// stdout
	ASSERT(__wasi_fd_fdstat_set_rights(1, 0, __WASI_RIGHTS_FD_WRITE) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(1, __WASI_RIGHTS_FD_WRITE, 0) == 0);
	ASSERT(__wasi_fd_fdstat_set_rights(1, __WASI_RIGHTS_FD_READ, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(1, __WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(1, __WASI_RIGHTS_FD_DATASYNC, 0) != 0);

	// stderr
	ASSERT(__wasi_fd_fdstat_set_rights(2, 0, __WASI_RIGHTS_FD_READ) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(2, __WASI_RIGHTS_FD_WRITE, 0) == 0);
	ASSERT(__wasi_fd_fdstat_set_rights(2, __WASI_RIGHTS_FD_READ, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(2, __WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(2, __WASI_RIGHTS_FD_SEEK, 0) != 0);

	// nonexistent
	ASSERT(__wasi_fd_fdstat_set_rights(3, 0, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(3, __WASI_RIGHTS_FD_READ, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(3, 0, __WASI_RIGHTS_FD_WRITE) != 0);

	// gate
	ASSERT(__wasi_fd_fdstat_set_rights(4, 0, __WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(4, __WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE, 0) == 0);
	ASSERT(__wasi_fd_fdstat_set_rights(4, __WASI_RIGHTS_FD_READ | __WASI_RIGHTS_FD_WRITE | __WASI_RIGHTS_FD_TELL, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(4, __WASI_RIGHTS_FD_TELL, 0) != 0);

	// nonexistent
	ASSERT(__wasi_fd_fdstat_set_rights(5, 0, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(5, __WASI_RIGHTS_FD_READ, 0) != 0);
	ASSERT(__wasi_fd_fdstat_set_rights(5, 0, __WASI_RIGHTS_FD_WRITE) != 0);
}

TEST_TRAP(fd_fdstat_set_rights_stdout_drop)
{
	(void) __wasi_fd_fdstat_set_rights(1, 0, 0);
}

TEST_TRAP(fd_fdstat_set_rights_stderr_drop)
{
	(void) __wasi_fd_fdstat_set_rights(2, 0, 0);
}

TEST_TRAP(fd_fdstat_set_rights_gate_drop_r)
{
	(void) __wasi_fd_fdstat_set_rights(4, __WASI_RIGHTS_FD_WRITE, 0);
}

TEST_TRAP(fd_fdstat_set_rights_gate_drop_w)
{
	(void) __wasi_fd_fdstat_set_rights(4, __WASI_RIGHTS_FD_READ, 0);
}

TEST_TRAP(fd_fdstat_set_rights_gate_drop_rw)
{
	(void) __wasi_fd_fdstat_set_rights(4, 0, 0);
}

TEST(fd_filestat_get)
{
	__wasi_filestat_t buf;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_filestat_get(fd, &buf) != 0);
}

TEST(fd_filestat_set_size)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_filestat_set_size(fd, 512) != 0);
}

TEST(fd_filestat_set_times)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_filestat_set_times(fd, 6000000000, 0, __WASI_FSTFLAGS_ATIM | __WASI_FSTFLAGS_MTIM_NOW) != 0);
}

TEST(fd_pread)
{
	__wasi_iovec_t iov;
	size_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_pread(fd, &iov, 1, 0, &len) != 0);
}

TEST(fd_prestat_dir_name)
{
	uint8_t buf[4096];
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_prestat_dir_name(fd, buf, sizeof buf) != 0);
}

TEST(fd_prestat_get)
{
	__wasi_prestat_t buf;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_prestat_get(fd, &buf) != 0);
}

TEST(fd_pwrite)
{
	__wasi_ciovec_t iov;
	size_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_pwrite(fd, &iov, 1, 0, &len) != 0);
}

TEST(fd_read)
{
	uint8_t buf[4][64];
	__wasi_iovec_t iov[4] = {
		{buf[0], sizeof buf[0]},
		{buf[1], sizeof buf[1]},
		{buf[2], sizeof buf[2]},
		{buf[3], sizeof buf[3]},
	};
	size_t len = 0;
	ASSERT(__wasi_fd_read(0, iov, 4, &len) != 0);
	ASSERT(__wasi_fd_read(1, iov, 4, &len) != 0);
	ASSERT(__wasi_fd_read(2, iov, 4, &len) != 0);
	ASSERT(__wasi_fd_read(3, iov, 4, &len) != 0);
	ASSERT(__wasi_fd_read(5, iov, 4, &len) != 0);
}

TEST(fd_readdir)
{
	uint8_t buf[1024];
	size_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_readdir(fd, buf, sizeof buf, 0, &len) != 0);
}

TEST(fd_renumber)
{
	ASSERT(__wasi_fd_renumber(0, 0) == 0);
	ASSERT(__wasi_fd_renumber(1, 1) == 0);
	ASSERT(__wasi_fd_renumber(2, 2) == 0);
	ASSERT(__wasi_fd_renumber(3, 3) != 0);
	ASSERT(__wasi_fd_renumber(__GATE_FD(), __GATE_FD()) == 0);
	ASSERT(__wasi_fd_renumber(5, 5) != 0);

	for (int fd = 0; fd < 10; fd++) {
		ASSERT(__wasi_fd_renumber(fd, 10) != 0);
		ASSERT(__wasi_fd_renumber(10, fd) != 0);
	}
}

TEST_TRAP(fd_renumber_stdin)
{
	(void) __wasi_fd_renumber(0, 1);
}

TEST_TRAP(fd_renumber_stdout)
{
	(void) __wasi_fd_renumber(1, __GATE_FD());
}

TEST_TRAP(fd_renumber_stderr)
{
	(void) __wasi_fd_renumber(2, 0);
}

TEST_TRAP(fd_renumber_gate)
{
	(void) __wasi_fd_renumber(__GATE_FD(), 1);
}

TEST(fd_seek)
{
	__wasi_filesize_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_seek(fd, 0, __WASI_WHENCE_END, &len) != 0);
}

TEST(sync)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_sync(fd) != 0);
}

TEST(fd_tell)
{
	__wasi_filesize_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_fd_tell(fd, &len) != 0);
}

TEST(fd_write)
{
	__wasi_ciovec_t iov[1] = {
		{(uint8_t*) "x", 1},
	};
	size_t len = 0;
	ASSERT(__wasi_fd_write(0, iov, 1, &len) != 0);
	ASSERT(__wasi_fd_write(3, iov, 1, &len) != 0);
	ASSERT(__wasi_fd_write(5, iov, 1, &len) != 0);
}

TEST(fd_write_stdout)
{
	__wasi_ciovec_t iov[3] = {
		{(uint8_t*) "PAS", 3},
		{(uint8_t*) "", 0},
		{(uint8_t*) "S\n", 2},
	};
	size_t len = 0;
	ASSERT(__wasi_fd_write(1, iov, 3, &len) == 0);
	ASSERT(len == 5);

	gate_exit(0); // Prevent duplicate output.
}

TEST(fd_write_stderr)
{
	__wasi_ciovec_t iov[1] = {
		{(uint8_t*) "PASS\n", 5},
	};
	size_t len = 0;
	ASSERT(__wasi_fd_write(2, iov, 1, &len) == 0);
	ASSERT(len == 5);

	gate_exit(0); // Prevent duplicate output.
}

TEST(fd_write_and_read_gate)
{
	// Write packet.
	{
		const struct gate_packet header = {
			.size = 8 + 2 + 1 + 5,
			.code = GATE_PACKET_CODE_SERVICES,
			.domain = GATE_PACKET_DOMAIN_CALL,
		};
		const uint8_t buf[3] = {1, 0, 5};
		__wasi_ciovec_t iov[3] = {
			{(uint8_t*) &header, 8},
			{buf, sizeof buf},
			{(uint8_t*) "bogus", 5},
		};
		size_t len = 0;
		ASSERT(__wasi_fd_write(__GATE_FD(), iov, 3, &len) == 0);
		ASSERT(len == 8 + 2 + 1 + 5);
	}

	// Read reply packet.
	while (1) {
		uint8_t buf[65536];
		__wasi_iovec_t iov = {buf, sizeof buf};
		size_t len = 0;
		__wasi_errno_t err = __wasi_fd_read(__GATE_FD(), &iov, 1, &len);

		if (err == __WASI_ERRNO_AGAIN)
			continue;

		ASSERT(err == 0);
		ASSERT(len == 16);
		break;
	}
}

TEST(io)
{
	struct gate_packet send_header = {
		.size = 8 + 2 + 1 + 5,
		.code = GATE_PACKET_CODE_SERVICES,
		.domain = GATE_PACKET_DOMAIN_CALL,
	};
	uint8_t send_buf[3] = {1, 0, 5};
	struct gate_iovec send_iov[3] = {
		{&send_header, 8},
		{send_buf, sizeof send_buf},
		{(void*) "bogus", 5},
	};
	unsigned send_num = 3;

	char recv_buf[65536];
	struct gate_iovec recv_iov[1] = {
		{recv_buf, sizeof recv_buf},
	};
	unsigned recv_num = 1;

	while (send_num || recv_num) {
		size_t received;
		size_t sent;
		gate_flags_t flags = ~0ULL;
		gate_io(recv_iov, recv_num, &received, send_iov, send_num, &sent, -1, &flags);

		if (sent) {
			ASSERT(sent == 16);
			send_num = 0;
		}

		if (received) {
			ASSERT(received == 16);
			recv_num = 0;
		}

		ASSERT(flags == 0);
	}
}

TEST(path_create_directory)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_create_directory(fd, STRARG("foo")) != 0);
}

TEST(path_filestat_get)
{
	__wasi_filestat_t buf;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_filestat_get(fd, __WASI_LOOKUPFLAGS_SYMLINK_FOLLOW, STRARG("bar"), &buf) != 0);
}

TEST(path_filestat_set_times)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_filestat_set_times(fd, __WASI_LOOKUPFLAGS_SYMLINK_FOLLOW, STRARG("foo"), 6000000000, 7000000000, __WASI_FSTFLAGS_ATIM | __WASI_FSTFLAGS_MTIM) != 0);
}

TEST(path_link)
{
	for (int fd = 0; fd < 10; fd++)
		for (int fd2 = 0; fd2 < 10; fd2++)
			ASSERT(__wasi_path_link(fd, 0, STRARG("foo"), fd2, STRARG("bar")) != 0);
}

TEST(path_open)
{
	int filefd = 0;
	for (int dirfd = 0; dirfd < 10; dirfd++)
		ASSERT(__wasi_path_open(dirfd, 0, STRARG("foo"), __WASI_OFLAGS_CREAT, __WASI_RIGHTS_FD_READ, 0, __WASI_FDFLAGS_APPEND, &filefd) != 0);
}

TEST(path_readlink)
{
	uint8_t buf[4096];
	size_t len = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_readlink(fd, STRARG("foo"), buf, sizeof buf, &len) != 0);
}

TEST(path_remove_directory)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_remove_directory(fd, STRARG("foo")) != 0);
}

TEST(path_rename)
{
	for (int fd = 0; fd < 10; fd++)
		for (int fd2 = 0; fd2 < 10; fd2++)
			ASSERT(__wasi_path_rename(fd, STRARG("foo"), fd2, STRARG("bar")) != 0);
}

TEST(path_symlink)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_symlink(STRARG("foo"), fd, STRARG("bar")) != 0);
}

TEST(path_unlink_file)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_path_unlink_file(fd, STRARG("bar")) != 0);
}

TEST(poll_oneoff)
{
	// Timing.
	{
		__wasi_subscription_t subs[4] = {
			{0, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_REALTIME, 1, 1, __WASI_SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME}}}},
			{1, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_REALTIME, 1 << 30, 1, 0}}}},
			{2, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_MONOTONIC, 0, 1, 0}}}},
			{3, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_MONOTONIC, 1ULL << 63, 1, __WASI_SUBCLOCKFLAGS_SUBSCRIPTION_CLOCK_ABSTIME}}}},
		};
		__wasi_event_t evs[4];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 4, &count) == 0);
		ASSERT(count == 2);

		bool ok[4] = {false, true, false, true};

		for (unsigned i = 0; i < count; i++) {
			unsigned id = evs[i].userdata;

			switch (id) {
			case 0:
			case 2:
				ASSERT(evs[i].error == 0);
				break;
			default:
				ASSERT(false);
				break;
			}

			ok[id] = true;
		}

		for (unsigned id = 0; id < 4; id++)
			ASSERT(ok[id]);
	}

	// Invalid clock id.
	{
		__wasi_subscription_t subs[1] = {
			{0, {__WASI_EVENTTYPE_CLOCK, {.clock = {4, 0, 1, 0}}}},
		};
		__wasi_event_t evs[1];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 1, &count) == 0);
		ASSERT(count == 1);

		for (unsigned i = 0; i < count; i++)
			ASSERT(evs[i].error != 0);
	}

	// Writable?
	{
		__wasi_subscription_t subs[6] = {
			{0, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {0}}}},
			{1, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {1}}}},
			{2, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {int(__GATE_FD())}}}},
			{3, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {2}}}},
			{4, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {3}}}},
			{5, {__WASI_EVENTTYPE_FD_WRITE, {.fd_write = {5}}}},
		};
		__wasi_event_t evs[6];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 6, &count) == 0);
		ASSERT(count == 6);

		bool ok[6] = {false, false, false, false, false, false};

		for (unsigned i = 0; i < count; i++) {
			unsigned id = evs[i].userdata;

			switch (id) {
			case 0: // stdin
			case 4: // fd 3
			case 5: // fd 5
				ASSERT(evs[i].error != 0);
				break;
			case 1: // stdout
			case 2: // gate
			case 3: // stderr
				ASSERT(evs[i].error == 0);
				ASSERT(evs[i].type == __WASI_EVENTTYPE_FD_WRITE);
				ASSERT(evs[i].fd_readwrite.nbytes > 0);
				ASSERT(evs[i].fd_readwrite.flags == 0);
				break;
			default:
				ASSERT(false);
				break;
			}

			ok[id] = true;
		}

		for (unsigned id = 0; id < count; id++)
			ASSERT(ok[id]);
	}

	// Readable?
	{
		__wasi_subscription_t subs[4] = {
			{0, {__WASI_EVENTTYPE_FD_READ, {.fd_read = {0}}}},
			{1, {__WASI_EVENTTYPE_FD_READ, {.fd_read = {1}}}},
			{2, {__WASI_EVENTTYPE_FD_READ, {.fd_read = {2}}}},
			{3, {__WASI_EVENTTYPE_FD_READ, {.fd_read = {int(__GATE_FD())}}}},
		};
		__wasi_event_t evs[4];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 4, &count) == 0);
		ASSERT(count == 3);

		bool ok[4] = {false, false, false, true};

		for (unsigned i = 0; i < count; i++) {
			unsigned id = evs[i].userdata;

			switch (id) {
			case 0: // stdin
			case 1: // stdout
			case 2: // stderr
				ASSERT(evs[i].error != 0);
				break;
			default:
				ASSERT(false);
				break;
			}

			ok[id] = true;
		}

		for (unsigned id = 0; id < 4; id++)
			ASSERT(ok[id]);
	}

	// Write packet.
	{
		const struct gate_packet header = {
			.size = 8 + 2 + 1 + 5,
			.code = GATE_PACKET_CODE_SERVICES,
			.domain = GATE_PACKET_DOMAIN_CALL,
		};
		const uint8_t buf[3] = {1, 0, 5};
		__wasi_ciovec_t iov[3] = {
			{(uint8_t*) &header, 8},
			{buf, sizeof buf},
			{(uint8_t*) "bogus", 5},
		};
		size_t len = 0;
		ASSERT(__wasi_fd_write(__GATE_FD(), iov, 3, &len) == 0);
		ASSERT(len == 8 + 2 + 1 + 5);
	}

	// Block on read.
	{
		__wasi_subscription_t subs[1] = {
			{0, {__WASI_EVENTTYPE_FD_READ, {.fd_read = {int(__GATE_FD())}}}},
		};
		__wasi_event_t evs[1];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 1, &count) == 0);
		ASSERT(count == 1);

		ASSERT(evs[0].userdata == 0);
		ASSERT(evs[0].error == 0);
		ASSERT(evs[0].type == __WASI_EVENTTYPE_FD_READ);
		ASSERT(evs[0].fd_readwrite.nbytes > 0);
		ASSERT(evs[0].fd_readwrite.flags == 0);
	}

	// Invalid event type.
	{
		__wasi_subscription_t subs[1] = {
			{0, {.tag = 100}},
		};
		__wasi_event_t evs[1];
		size_t count = 99;
		ASSERT(__wasi_poll_oneoff(subs, evs, 1, &count) == 0);
		ASSERT(count == 1);

		for (unsigned i = 0; i < count; i++)
			ASSERT(evs[i].error != 0);
	}
}

TEST_TRAP(poll_oneoff_process_cputime)
{
	__wasi_subscription_t subs[1] = {
		{0, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_PROCESS_CPUTIME_ID, 1, 1, 0}}}},
	};
	__wasi_event_t evs[1];
	size_t count = 99;
	(void) __wasi_poll_oneoff(subs, evs, 1, &count);
}

TEST_TRAP(poll_oneoff_thread_cputime)
{
	__wasi_subscription_t subs[1] = {
		{0, {__WASI_EVENTTYPE_CLOCK, {.clock = {__WASI_CLOCKID_THREAD_CPUTIME_ID, 1, 1, 0}}}},
	};
	__wasi_event_t evs[1];
	size_t count = 99;
	(void) __wasi_poll_oneoff(subs, evs, 1, &count);
}

TEST_TRAP(proc_raise)
{
	(void) __wasi_proc_raise(1);
}

TEST(random_get)
{
	uint8_t buf[16] = {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0};
	ASSERT(__wasi_random_get(buf, sizeof buf) == 0);

	for (unsigned i = 0; i < sizeof buf; i++) {
		if (buf[i] != 0)
			return;
	}
	ASSERT(0);
}

TEST_TRAP(random_get_too_much)
{
	uint8_t buf[17];
	(void) __wasi_random_get(buf, sizeof buf);
}

TEST(sched_yield)
{
	ASSERT(__wasi_sched_yield() == 0);
}

TEST(sock_recv)
{
	__wasi_iovec_t iov;
	size_t count = 0;
	__wasi_roflags_t flags = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_sock_recv(fd, &iov, 1, __WASI_RIFLAGS_RECV_WAITALL, &count, &flags) != 0);
}

TEST(sock_send)
{
	__wasi_ciovec_t iov;
	size_t count = 0;
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_sock_send(fd, &iov, 1, 0, &count) != 0);
}

TEST(sock_shutdown)
{
	for (int fd = 0; fd < 10; fd++)
		ASSERT(__wasi_sock_shutdown(fd, __WASI_SDFLAGS_WR) != 0);
}

extern "C" void* memcpy(void* dest, void const* src, size_t n)
{
	auto d = reinterpret_cast<uint8_t*>(dest);
	auto s = reinterpret_cast<uint8_t const*>(src);
	for (size_t i = 0; i < n; i++)
		d[i] = s[i];
	return dest;
}

extern "C" void* memset(void* s, int c, size_t n)
{
	for (size_t i = 0; i < n; i++)
		reinterpret_cast<uint8_t*>(s)[i] = c;
	return s;
}
