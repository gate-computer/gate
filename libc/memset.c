#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

void __gate_memset_p0i8_i32(uint8_t *dest, uint8_t val, uint32_t len, uint32_t align, bool isvolatile);

void *memset(void *s, int c, size_t n)
{
	__gate_memset_p0i8_i32(s, c, n, 1, false);
	return s;
}
