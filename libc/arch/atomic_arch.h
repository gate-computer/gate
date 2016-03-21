#define a_cas a_cas
static inline int a_cas(volatile int *p, int t, int s)
{
	int old = *p;
	if (old == t)
		*p = s;
	return old;
}

#define a_swap a_swap
static inline int a_swap(volatile int *p, int v)
{
	int old = *p;
	*p = v;
	return old;
}

#define a_fetch_add a_fetch_add
static inline int a_fetch_add(volatile int *p, int v)
{
	int old = *p;
	*p = (unsigned) old + v;
	return old;
}

#define a_fetch_and a_fetch_and
static inline int a_fetch_and(volatile int *p, int v)
{
	int old = *p;
	*p = old & v;
	return old;
}

#define a_fetch_or a_fetch_or
static inline int a_fetch_or(volatile int *p, int v)
{
	int old = *p;
	*p = old | v;
	return old;
}
