#include <stddef.h>
#include <stdlib.h>

#include <gate.h>

extern "C" {
	int *__errno_location()
	{
		static int errno_storage;
		return &errno_storage;
	}
}

namespace std {
	typedef ::size_t size_t;
	class bad_alloc {};
	struct nothrow_t {};
}

void *operator new(std::size_t size) throw (std::bad_alloc)
{
	void *ptr = malloc(size);
	if (ptr == nullptr)
		gate_exit(1);
	return ptr;
}

void *operator new(std::size_t size, const std::nothrow_t &nothrow) throw ()
{
	return malloc(size);
}

void *operator new(std::size_t size, void *ptr) throw ()
{
	return ptr;
}

void *operator new[](std::size_t size) throw (std::bad_alloc)
{
	void *ptr = malloc(size);
	if (ptr == nullptr)
		gate_exit(1);
	return ptr;
}

void *operator new[](std::size_t size, const std::nothrow_t &nothrow) throw ()
{
	return malloc(size);
}

void *operator new[](std::size_t size, void *ptr) throw ()
{
	return ptr;
}

void operator delete(void *ptr) throw ()
{
	free(ptr);
}

void operator delete(void *ptr, const std::nothrow_t &nothrow) throw ()
{
	free(ptr);
}

void operator delete(void *ptr, void *buf) throw ()
{
}

void operator delete[](void *ptr) throw ()
{
	free(ptr);
}


void operator delete[](void *ptr, const std::nothrow_t &nothrow) throw ()
{
	free(ptr);
}

void operator delete[](void *ptr, void *buf) throw ()
{
}
