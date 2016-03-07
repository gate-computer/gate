#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <gate.h>

int main(int argc, char **argv);

GATE_NORETURN
void _start(void)
{
	static char *argv[] = { "a.out", NULL };
	gate_exit(main(1, argv));
}

void __gate_memcpy_p0i8_p0i8_i32(uint8_t *dest, const uint8_t *src, uint32_t n, uint32_t align, bool isvolatile)
{
	for (; n > 0; n--)
		*dest++ = *src++;
}

void __gate_memcpy_p0i8_p0i8_i64(uint8_t *dest, const uint8_t *src, uint64_t n, uint32_t align, bool isvolatile)
{
	__gate_memcpy_p0i8_p0i8_i32(dest, src, n, align, isvolatile);
}
