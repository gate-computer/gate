#include <stddef.h>

#include <gate.h>

int main(void);

GATE_NORETURN
void __start(void)
{
	gate_exit(main());
}
