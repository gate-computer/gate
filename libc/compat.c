int *__errno_location(void)
{
	static int errno_storage;
	return &errno_storage;
}
