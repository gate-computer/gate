// +build linux

package memfd

const (
	_F_LINUX_SPECIFIC_BASE = 1024

	F_ADD_SEALS = FcntlCmd(_F_LINUX_SPECIFIC_BASE + 9)
	F_GET_SEALS = FcntlCmd(_F_LINUX_SPECIFIC_BASE + 10)
)
