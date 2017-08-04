#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

int main()
{
	const char *str = "ok\n";

	char *buf = malloc(strlen(str) + 1);
	if (buf == NULL)
		gate_exit(1);

	strcpy(buf, str);
	gate_debug(buf);
	return 0;
}
