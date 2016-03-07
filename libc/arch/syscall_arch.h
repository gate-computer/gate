#include <bits/errno.h>

#define __SYSCALL_LL_E(x) (x)
#define __SYSCALL_LL_O(x) (x)

static __inline long __syscall0(long n)
{
	return -ENOSYS;
}

static __inline long __syscall1(long n, long a1)
{
	return -ENOSYS;
}

static __inline long __syscall2(long n, long a1, long a2)
{
	return -ENOSYS;
}

static __inline long __syscall3(long n, long a1, long a2, long a3)
{
	return -ENOSYS;
}

static __inline long __syscall4(long n, long a1, long a2, long a3, long a4)
{
	return -ENOSYS;
}

static __inline long __syscall5(long n, long a1, long a2, long a3, long a4, long a5)
{
	return -ENOSYS;
}

static __inline long __syscall6(long n, long a1, long a2, long a3, long a4, long a5, long a6)
{
	return -ENOSYS;
}
