#include <stddef.h>

#include <gate.h>

int main(int argc, char **argv);

GATE_NORETURN
void _start(void)
{
	static char *argv[] = { "a.out", NULL };
	gate_exit(main(1, argv));
}
