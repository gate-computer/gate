#include <gate.h>

void (*indirection)(void);

static void implementation(void)
{
	char s[] = "hello world\n";
	gate_send(0, s, sizeof (s) - 1);
}

int main(int argc, char **argv)
{
	indirection = implementation;
	indirection();
	return 0;
}
