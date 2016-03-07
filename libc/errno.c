int *__errno_location(void)
{
	static int errno_val;
	return &errno_val;
}
