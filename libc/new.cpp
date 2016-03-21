#include <stddef.h>
#include <stdlib.h>

void *operator new(size_t size)
{
	return malloc(size);
}

void *operator new[](size_t size)
{
	return malloc(size);
}

void operator delete(void *ptr) throw ()
{
	free(ptr);
}

void operator delete[](void *ptr) throw ()
{
	free(ptr);
}
