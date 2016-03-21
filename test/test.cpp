#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

void (*indirection)(const char *str, size_t len);

namespace {

void implementation(const char *str, size_t len)
{
	gate_send(0, str, len);
}

class ScopedBuf
{
	ScopedBuf(const ScopedBuf &) = delete;
	ScopedBuf &operator=(const ScopedBuf &) = delete;

public:
	explicit ScopedBuf(size_t size): ptr(new char[size]) {}
	~ScopedBuf() { delete[] ptr; }

	operator bool() { return ptr != nullptr; }

	char *const ptr;
};

} // namespace

int main(int argc, char **argv)
{
	indirection = implementation;

	auto dummy = new int(42);
	if (dummy == nullptr)
		return EXIT_FAILURE;

	delete dummy;

	ScopedBuf buf(10000);
	if (!buf)
		return EXIT_FAILURE;

	char str[] = "hello world\n";
	memcpy(buf.ptr, str, sizeof (str));

	indirection(buf.ptr, sizeof (str) - 1);

	return EXIT_SUCCESS;
}
