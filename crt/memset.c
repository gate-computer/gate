#include <stdbool.h>
#include <stdint.h>

void __gate_memset_p0i8_i32(uint8_t *dest, uint8_t val, uint32_t len, uint32_t align, bool isvolatile)
{
	for (; len > 0; len--)
		*dest++ = val;
}

void __gate_memset_p0i8_i64(uint8_t *dest, uint8_t val, uint64_t len, uint32_t align, bool isvolatile)
{
	__gate_memset_p0i8_i32(dest, val, len, align, isvolatile);
}
