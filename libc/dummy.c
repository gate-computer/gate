void *__mmap()
{
	return (void *) -1;
}

int __munmap()
{
	return -1;
}

void *__mremap()
{
	return (void *) -1;
}

int __madvise()
{
	return 0;
}

void __wait()
{
}
