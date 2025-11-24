//go:build !unix

package executor

func applySecureUmask() func() { return nil }
