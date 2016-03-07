#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

void __gate_memcpy_p0i8_p0i8_i32(uint8_t *dest, const uint8_t *src, uint32_t n, uint32_t align, bool isvolatile);

void *memcpy(void *dest, const void *src, size_t n)
{
	__gate_memcpy_p0i8_p0i8_i32(dest, src, n, 1, false);
	return dest;
}
