#include <stddef.h>
#include <stdlib.h>
#include <string.h>

#include <gate.h>

void (*indirection)(const char *str, size_t len);

static void implementation(const char *str, size_t len)
{
	gate_send(0, str, len);
}

int main(int argc, char **argv)
{
	indirection = implementation;

	void *ptr = malloc(10000);
	if (!ptr)
		return 1;

	char str[] = "hello world\n";
	memcpy(ptr, str, sizeof (str));

	indirection(reinterpret_cast<char *> (ptr), sizeof (str) - 1);

	free(ptr);

	return 0;
}
