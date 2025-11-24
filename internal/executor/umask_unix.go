//go:build unix

package executor

import "syscall"

func applySecureUmask() func() {
	old := syscall.Umask(0o077)
	return func() { syscall.Umask(old) }
}
