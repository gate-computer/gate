#include <stdbool.h>
#include <stdint.h>

void __gate_memcpy_p0i8_p0i8_i32(uint8_t *dest, const uint8_t *src, uint32_t n, uint32_t align, bool isvolatile)
{
	for (; n > 0; n--)
		*dest++ = *src++;
}

void __gate_memcpy_p0i8_p0i8_i64(uint8_t *dest, const uint8_t *src, uint64_t n, uint32_t align, bool isvolatile)
{
	__gate_memcpy_p0i8_p0i8_i32(dest, src, n, align, isvolatile);
}
