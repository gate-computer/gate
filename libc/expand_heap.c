#include <stddef.h>

#include <gate.h>

void *__expand_heap(size_t *pn)
{
	static size_t alloc;

	size_t new_alloc = alloc + *pn;
	if (new_alloc > gate_heap_size)
		return NULL;

	void *ptr = (void *) (gate_heap_addr + alloc);
	alloc = new_alloc;
	return ptr;
}
