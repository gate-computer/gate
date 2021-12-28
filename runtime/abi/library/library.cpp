// Copyright (c) 2021 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Update ../library.go by running 'go generate' in parent directory.

#include <stddef.h>
#include <stdint.h>

#include <rt.h>

#define EXPORT __attribute__((visibility("default")))

// Avoid inheritance to avoid stack access and globals.

#define FLAGS_CLASS_MEMBERS(Class, Primitive) \
public: \
	Class() {} \
	bool operator==(Class x) const { return m_flags == x.m_flags; } \
	Class operator|(Class x) const { return Class(m_flags | x.m_flags); } \
	bool contains_all(Class x) const { return (m_flags & x.m_flags) == x.m_flags; } \
	bool contains_any() const { return m_flags != 0; } \
	bool contains_none() const { return m_flags == 0; } \
	Class exclude(Class x) const { return Class(m_flags & ~x.m_flags); } \
\
private: \
	explicit Class(Primitive x): m_flags(x) {} \
	Primitive m_flags = 0;

#define RETURN_FAULT_IF(condition) \
	do { \
		if (condition) \
			return Error::fault; \
	} while (0)

// For some reason noreturn function didn't get inlined.
#define trap_abi_deficiency() rt_trap(127)

namespace {

enum class Error : uint16_t {
	success = 0,
	again = 6,
	badf = 8,
	fault = 21,
	inval = 28,
	notsock = 57,
	perm = 63,
	notcapable = 76,
};

class Timestamp {
public:
	static const Timestamp zero;
	static const Timestamp max;

	bool operator<(Timestamp x) const { return m_ns < x.m_ns; }
	bool operator<=(Timestamp x) const { return m_ns <= x.m_ns; }
	Timestamp operator+(Timestamp x) const { return Timestamp(m_ns + x.m_ns); }
	int64_t operator/(uint64_t x) const { return m_ns / x; }
	int64_t operator%(uint64_t x) const { return m_ns % x; }
	Timestamp operator-(Timestamp x) const { return Timestamp(m_ns - x.m_ns); }
	bool is_zero() const { return m_ns == 0; }
	bool is_nonzero() const { return m_ns != 0; }

private:
	explicit Timestamp(uint64_t ns):
		m_ns(ns) {}

	uint64_t m_ns;
};

const Timestamp Timestamp::zero = Timestamp(0);
const Timestamp Timestamp::max = Timestamp(~0ULL);

class Resolution {
	Resolution() = delete;

public:
	explicit Resolution(uint64_t ns = 0):
		m_ns(ns) {}

	uint64_t m_ns;
};

enum class FD : uint32_t {
	stdin = 0,
	stdout = 1,
	stderr = 2,
	gate = 4,
};

class PollEvents {
public:
	static const PollEvents input;
	static const PollEvents output;

	FLAGS_CLASS_MEMBERS(PollEvents, uint32_t)
};

const PollEvents PollEvents::input = PollEvents(1 << 0);
const PollEvents PollEvents::output = PollEvents(1 << 2);

class Rights {
public:
	static const Rights fd_read;
	static const Rights fd_write;

	FLAGS_CLASS_MEMBERS(Rights, uint64_t)
};

const Rights Rights::fd_read = Rights(1 << 1);
const Rights Rights::fd_write = Rights(1 << 6);

enum class ClockID : uint32_t {
	realtime = 0,
	monotonic = 1,
	realtime_coarse = 5,
	monotonic_coarse = 6,
};

class ClockFlags {
public:
	static const ClockFlags abstime;

	FLAGS_CLASS_MEMBERS(ClockFlags, uint16_t)
};

const ClockFlags ClockFlags::abstime = ClockFlags(1 << 0);

class FDFlags {
public:
	static const FDFlags nonblock;

	FLAGS_CLASS_MEMBERS(FDFlags, uint16_t)
};

const FDFlags FDFlags::nonblock = FDFlags(1 << 2);

enum class FileType : uint8_t {
	unknown = 0,
};

enum class EventType : uint8_t {
	clock = 0,
	fd_read = 1,
	fd_write = 2,
};

class EventRWFlags {
public:
	FLAGS_CLASS_MEMBERS(EventRWFlags, uint16_t)
};

struct IOVec {
	void* iov_base;
	uint32_t iov_len;
};

struct FDStat {
	FileType fs_filetype;
	FDFlags fs_flags;
	Rights fs_rights_base;
	Rights fs_rights_inheriting;
};

struct Subscription {
	uint64_t userdata;

	struct {
		EventType tag;

		union {
			struct {
				ClockID clockid;
				Timestamp timeout;
				Resolution precision;
				ClockFlags flags;
			} clock;

			struct {
				FD fd;
			} fd_readwrite;
		} u;
	};
};

struct Event {
	uint64_t userdata;
	Error error;
	EventType type;

	union {
		struct {
			uint64_t nbytes;
			EventRWFlags flags;
		} fd_readwrite;
	} u;
};

class Time {
public:
	Time() {}

	Timestamp get(ClockID id) const
	{
		if (id == ClockID::realtime)
			return realtime;
		else
			return monotonic;
	}

	void set(ClockID id, Timestamp t)
	{
		if (id == ClockID::realtime)
			realtime = t;
		else
			monotonic = t;
	}

	// Avoid stack memory access and globals by not using array.

	Timestamp realtime = Timestamp::zero;
	Timestamp monotonic = Timestamp::zero;
};

inline uint64_t bytes64(uint8_t a0, uint8_t a1 = 0, uint8_t a2 = 0, uint8_t a3 = 0, uint8_t a4 = 0, uint8_t a5 = 0, uint8_t a6 = 0, uint8_t a7 = 0)
{
	return (uint64_t(a0) << 0x00) |
	       (uint64_t(a1) << 0x08) |
	       (uint64_t(a2) << 0x10) |
	       (uint64_t(a3) << 0x18) |
	       (uint64_t(a4) << 0x20) |
	       (uint64_t(a5) << 0x28) |
	       (uint64_t(a6) << 0x30) |
	       (uint64_t(a7) << 0x38);
}

inline bool clock_is_valid(ClockID id)
{
	return uint32_t(id) < 4;
}

inline bool clock_is_supported(ClockID id)
{
	return uint32_t(id) < 2;
}

inline ClockID clock_to_coarse(ClockID id)
{
	if (uint32_t(id) < 2)
		return ClockID(uint32_t(id) + uint32_t(ClockID::realtime_coarse) - uint32_t(ClockID::realtime));

	return id;
}

inline Error fd_error(FD fd, Error err)
{
	if (fd == FD::gate || fd == FD::stdin || fd == FD::stdout || fd == FD::stderr)
		return err;

	return Error::badf;
}

} // namespace

extern "C" {

Timestamp rt_time(ClockID id);
uint32_t rt_timemask();
size_t rt_read(void* buf, size_t size);
size_t rt_write(void const* data, size_t size);
PollEvents rt_poll(PollEvents in, PollEvents out, int64_t nsec, int64_t sec); // Note order.
int rt_random();

} // extern "C"

namespace {

inline Resolution time_resolution()
{
	auto r = uint64_t(~rt_timemask()) + 1;
	if (r > 1000000000)
		r = 1000000000;

	return Resolution(r);
}

} // namespace

extern "C" {

EXPORT Error args_get(char** argv, char* argvbuf)
{
	return Error::success;
}

EXPORT Error args_sizes_get(int32_t* argc_ptr, uint32_t* argvbufsize_ptr)
{
	RETURN_FAULT_IF(argc_ptr == nullptr);
	RETURN_FAULT_IF(argvbufsize_ptr == nullptr);

	*argc_ptr = 0;
	*argvbufsize_ptr = 0;
	return Error::success;
}

EXPORT Error clock_res_get(ClockID id, Resolution* buf)
{
	RETURN_FAULT_IF(buf == nullptr);

	if (!clock_is_valid(id))
		return Error::inval;

	*buf = time_resolution();
	return Error::success;
}

EXPORT Error clock_time_get(ClockID id, Resolution precision, Timestamp* buf)
{
	RETURN_FAULT_IF(buf == nullptr);

	if (!clock_is_valid(id))
		return Error::inval;

	if (clock_is_supported(id)) {
		*buf = rt_time(clock_to_coarse(id));
		return Error::success;
	}

	trap_abi_deficiency();
}

EXPORT Error environ_get(void** env, uint64_t* buf)
{
	RETURN_FAULT_IF(env == nullptr);
	RETURN_FAULT_IF(buf == nullptr);

	buf[0] = bytes64('G', 'A', 'T', 'E', '_', 'A', 'B', 'I');
	buf[1] = bytes64('_', 'V', 'E', 'R', 'S', 'I', 'O', 'N');
	buf[2] = bytes64('=', '0', '\0');

	buf[3] = bytes64('G', 'A', 'T', 'E', '_', 'F', 'D', '=');
	buf[4] = bytes64('4', '\0');

	buf[5] = bytes64('G', 'A', 'T', 'E', '_', 'M', 'A', 'X');
	buf[6] = bytes64('_', 'S', 'E', 'N', 'D', '_', 'S', 'I');
	buf[7] = bytes64('Z', 'E', '=', '6', '5', '5', '3', '6');
	buf[8] = bytes64('\0');

	env[0] = &buf[0];
	env[1] = &buf[3];
	env[2] = &buf[5];

	return Error::success;
}

EXPORT Error environ_sizes_get(int32_t* envlen_ptr, uint32_t* envbufsize_ptr)
{
	RETURN_FAULT_IF(envlen_ptr == nullptr);
	RETURN_FAULT_IF(envbufsize_ptr == nullptr);

	*envlen_ptr = 3;
	*envbufsize_ptr = 9 * sizeof(uint64_t);
	return Error::success;
}

EXPORT FD fd()
{
	return FD::gate;
}

EXPORT Error fd_close(FD fd)
{
	if (fd == FD::gate || fd == FD::stdin || fd == FD::stdout || fd == FD::stderr)
		trap_abi_deficiency();

	return Error::badf;
}

EXPORT Error fd_fdstat_get(FD fd, FDStat* buf)
{
	RETURN_FAULT_IF(buf == nullptr);

	FDFlags flags;
	Rights rights;

	if (fd == FD::gate) {
		flags = FDFlags::nonblock;
		rights = Rights::fd_read | Rights::fd_write;
	} else if (fd == FD::stdout || fd == FD::stderr) {
		rights = Rights::fd_write;
	} else if (fd != FD::stdin) {
		return Error::badf;
	}

	buf->fs_filetype = FileType::unknown;
	buf->fs_flags = flags;
	buf->fs_rights_base = rights;
	buf->fs_rights_inheriting = Rights();
	return Error::success;
}

EXPORT Error fd_fdstat_set_rights(FD fd, Rights base, Rights inheriting)
{
	if (fd == FD::gate) {
		if (inheriting.contains_any())
			return Error::notcapable;

		if (base == (Rights::fd_read | Rights::fd_write))
			return Error::success;

		if (base.exclude(Rights::fd_read | Rights::fd_write).contains_any())
			return Error::notcapable;

		trap_abi_deficiency();
	}

	if (fd == FD::stdout || fd == FD::stderr) {
		if (inheriting.contains_any())
			return Error::notcapable;

		if (base == Rights::fd_write)
			return Error::success;

		if (base.contains_any())
			return Error::notcapable;

		trap_abi_deficiency();
	}

	if (fd == FD::stdin) {
		if (inheriting.contains_any())
			return Error::notcapable;

		if (base.contains_none())
			return Error::success;

		return Error::notcapable;
	}

	return Error::badf;
}

EXPORT Error fd_prestat_dir_name(FD fd, char* buf, size_t bufsize)
{
	RETURN_FAULT_IF(bufsize > 0 && buf == nullptr);

	return fd_error(fd, Error::inval);
}

EXPORT Error fd_read(FD fd, IOVec const* iov, int iovlen, uint32_t* nread_ptr)
{
	RETURN_FAULT_IF(iovlen > 0 && iov == nullptr);
	RETURN_FAULT_IF(nread_ptr == nullptr);

	if (fd == FD::gate) {
		size_t total = 0;

		for (int i = 0; i < iovlen; i++) {
			auto len = iov[i].iov_len;
			auto n = rt_read(iov[i].iov_base, len);
			total += n;
			if (n < len) {
				if (total == 0)
					return Error::again;
				break;
			}
		}

		*nread_ptr = total;
		return Error::success;
	}

	if (fd == FD::stdin || fd == FD::stdout || fd == FD::stderr)
		return Error::perm;

	return Error::badf;
}

EXPORT Error fd_renumber(FD from, FD to)
{
	if (from == FD::stdin || from == FD::stdout || from == FD::stderr || from == FD::gate) {
		if (to == FD::stdin || to == FD::stdout || to == FD::stderr || to == FD::gate) {
			if (from == to)
				return Error::success;

			trap_abi_deficiency();
		}
	}

	return Error::badf;
}

EXPORT Error fd_write(FD fd, IOVec const* iov, int iovlen, uint32_t* nwritten_ptr)
{
	RETURN_FAULT_IF(iovlen > 0 && iov == nullptr);
	RETURN_FAULT_IF(nwritten_ptr == nullptr);

	size_t total = 0;

	if (fd == FD::gate) {
		for (int i = 0; i < iovlen; i++) {
			auto len = iov[i].iov_len;
			auto n = rt_write(iov[i].iov_base, len);
			total += n;
			if (n < len) {
				if (total == 0)
					return Error::again;
				break;
			}
		}
	} else if (fd == FD::stdout || fd == FD::stderr) {
		for (int i = 0; i < iovlen; i++) {
			auto len = iov[i].iov_len;
			rt_debug(iov[i].iov_base, len);
			total += len;
		}
	} else if (fd == FD::stdin) {
		return Error::perm;
	} else {
		return Error::badf;
	}

	*nwritten_ptr = total;
	return Error::success;
}

EXPORT void io(IOVec const* recv, int recvlen, uint32_t* nrecv_ptr, IOVec const* send, int sendlen, uint32_t* nsent_ptr, int64_t timeout)
{
	auto events = PollEvents::input | PollEvents::output;

	bool sending = false;
	for (int i = 0; i < sendlen; i++) {
		if (send[i].iov_len > 0) {
			sending = true;
			break;
		}
	}

	// Don't bother with sub-microsecond wait, unless it's the only task.
	if (timeout >= 0 && timeout < 1000) {
		if (sending)
			goto no_wait;

		for (int i = 0; i < recvlen; i++) {
			if (recv[i].iov_len > 0)
				goto no_wait;
		}
	}

	{
		int64_t sec = -1;
		int64_t nsec = 0;
		if (timeout >= 0) {
			sec = timeout / 1000000000LL;
			nsec = timeout % 1000000000LL;
		}

		PollEvents out;
		if (sending)
			out = PollEvents::output;

		events = rt_poll(PollEvents::input, out, nsec, sec);
	}

no_wait:;
	size_t nsent = 0;
	size_t nrecv = 0;

	if (events.contains_all(PollEvents::output)) {
		for (int i = 0; i < sendlen; i++) {
			auto len = send[i].iov_len;
			auto n = rt_write(send[i].iov_base, len);
			nsent += n;
			if (n < len)
				break;
		}
	}

	if (events.contains_all(PollEvents::input)) {
		for (int i = 0; i < recvlen; i++) {
			auto len = recv[i].iov_len;
			auto n = rt_read(recv[i].iov_base, len);
			nrecv += n;
			if (n < len)
				break;
		}
	}

	if (nsent_ptr)
		*nsent_ptr = nsent;
	if (nrecv_ptr)
		*nrecv_ptr = nrecv;
}

EXPORT Error poll_oneoff(Subscription const* sub, Event* out, int nsub, uint32_t* nout_ptr)
{
	RETURN_FAULT_IF(nsub > 0 && sub == nullptr);
	RETURN_FAULT_IF(nsub > 0 && out == nullptr);
	RETURN_FAULT_IF(nout_ptr == nullptr);

	PollEvents pollin;
	PollEvents pollout;
	bool have_timeout = false;
	auto timeout = Timestamp::max;
	Time begin;

	for (int i = 0; i < nsub; i++) {
		if (sub[i].tag == EventType::clock) {
			auto id = sub[i].u.clock.clockid;

			if (clock_is_valid(id)) {
				if (!clock_is_supported(id))
					trap_abi_deficiency();

				auto t = sub[i].u.clock.timeout;

				if (t.is_nonzero()) { // Optimize special case.
					if (begin.get(id).is_zero())
						begin.set(id, rt_time(clock_to_coarse(id)));
				}

				if (sub[i].u.clock.flags.contains_all(ClockFlags::abstime)) {
					auto now = begin.get(id);
					if (t < now)
						t = now;
					t = t - now;
				}

				if (t < timeout)
					timeout = t;
				have_timeout = true;
				continue;
			}
		} else if (sub[i].tag == EventType::fd_read) {
			if (sub[i].u.fd_readwrite.fd == FD::gate) {
				pollin = PollEvents::input;
				continue;
			}
		} else if (sub[i].tag == EventType::fd_write) {
			if (sub[i].u.fd_readwrite.fd == FD::gate) {
				pollout = PollEvents::output;
				continue;
			}
		}

		timeout = Timestamp::zero;
		have_timeout = true;
	}

	int64_t sec = -1;
	int64_t nsec = 0;
	if (have_timeout) {
		sec = timeout / 1000000000ULL;
		nsec = timeout % 1000000000ULL;
	}

	auto r = rt_poll(pollin, pollout, nsec, sec);

	Time end;
	if (begin.realtime.is_nonzero())
		end.realtime = rt_time(ClockID::realtime_coarse);
	if (begin.monotonic.is_nonzero())
		end.monotonic = rt_time(ClockID::monotonic_coarse);

	int n = 0;

	for (int i = 0; i < nsub; i++) {
		out[n].userdata = sub[i].userdata;
		out[n].error = Error::success;
		out[n].type = sub[i].tag;
		out[n].u.fd_readwrite.nbytes = 0;
		out[n].u.fd_readwrite.flags = EventRWFlags();

		if (sub[i].tag == EventType::clock) {
			auto id = sub[i].u.clock.clockid;

			if (clock_is_valid(id)) {
				auto t = sub[i].u.clock.timeout;

				if (t.is_zero()) { // Optimize special case.
					n++;
					continue;
				}

				if (!sub[i].u.clock.flags.contains_all(ClockFlags::abstime)) {
					auto abstime = end.get(id) + t;
					if (abstime < t) // Overflow?
						continue;
					t = abstime;
				}

				if (t <= end.get(id))
					n++;
				continue;
			}
		} else if (sub[i].tag == EventType::fd_read) {
			auto fd = sub[i].u.fd_readwrite.fd;

			if (fd == FD::gate) {
				if (r.contains_all(PollEvents::input)) {
					out[n].u.fd_readwrite.nbytes = 65536;
					n++;
				}
				continue;
			}

			if (fd == FD::stdin || fd == FD::stdout || fd == FD::stderr) {
				out[n].error = Error::perm;
				n++;
				continue;
			}

			out[n].error = Error::badf;
			n++;
			continue;
		} else if (sub[i].tag == EventType::fd_write) {
			auto fd = sub[i].u.fd_readwrite.fd;

			if (fd == FD::gate) {
				if (r.contains_all(PollEvents::output)) {
					out[n].u.fd_readwrite.nbytes = 65536;
					n++;
				}
				continue;
			}

			if (fd == FD::stdout || fd == FD::stderr) {
				out[n].u.fd_readwrite.nbytes = 0x7fffffff;
				n++;
				continue;
			}

			if (fd == FD::stdin) {
				out[n].error = Error::perm;
				n++;
				continue;
			}

			out[n].error = Error::badf;
			n++;
			continue;
		}

		out[n].error = Error::inval;
		n++;
	}

	*nout_ptr = n;
	return Error::success;
}

EXPORT void proc_exit(int status)
{
	// Terminating variants.
	rt_trap(status ? 3 : 2);
}

EXPORT Error proc_raise(int signal)
{
	trap_abi_deficiency();
}

EXPORT Error random_get(uint8_t* buf, size_t len)
{
	RETURN_FAULT_IF(len > 0 && buf == nullptr);

	while (len > 0) {
		auto value = rt_random();
		if (value >= 0) {
			*buf++ = uint8_t(value);
			len--;
		} else {
			trap_abi_deficiency();
		}
	}

	return Error::success;
}

EXPORT Error sched_yield()
{
	return Error::success;
}

EXPORT Error sock_recv(FD fd, int a1, int a2, int a3, int a4, int a5)
{
	if (fd == FD::gate)
		return Error::notsock;

	if (fd == FD::stdin || fd == FD::stdout || fd == FD::stderr)
		return Error::perm;

	return Error::badf;
}

EXPORT Error sock_send(FD fd, int a1, int a2, int a3, int a4)
{
	if (fd == FD::gate || fd == FD::stdout || fd == FD::stderr)
		return Error::notsock;

	if (fd == FD::stdin)
		return Error::perm;

	return Error::badf;
}

EXPORT Error stub_fd(FD fd)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32(FD fd, int a1)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i64(FD fd, int64_t a1)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32(FD fd, int a1, int a2)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i64_i64(FD fd, int64_t a1, int64_t a2)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i64_i32_i32(FD fd, int64_t a1, int a2, int a3)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i64_i64_i32(FD fd, int64_t a1, int64_t a2, int a3)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32_i32_i32(FD fd, int a1, int a2, int a3, int a4)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_i32_i32_fd_i32_i32(int a0, int a1, FD fd, int a3, int a4)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32_i64_i32(FD fd, int a1, int a2, int64_t a3, int a4)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32_fd_i32_i32(FD fd, int a1, int a2, FD fd3, int a4, int a5)
{
	return fd_error(fd, fd_error(fd3, Error::perm));
}

EXPORT Error stub_fd_i32_i32_i32_i32_i32(FD fd, int a1, int a2, int a3, int a4, int a5)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32_i32_fd_i32_i32(FD fd, int a1, int a2, int a3, FD fd4, int a5, int a6)
{
	return fd_error(fd, fd_error(fd4, Error::perm));
}

EXPORT Error stub_fd_i32_i32_i32_i64_i64_i32(FD fd, int a1, int a2, int a3, int64_t a4, int64_t a5, int a6)
{
	return fd_error(fd, Error::perm);
}

EXPORT Error stub_fd_i32_i32_i32_i32_i64_i64_i32_i32(FD fd, int a1, int a2, int a3, int a4, int64_t a5, int64_t a6, int a7, int a8)
{
	return fd_error(fd, Error::perm);
}

} // extern "C"
