// SPDX-License-Identifier: AGPL-3.0-or-later
package secrets

import "fmt"

type safeError struct {
	op  string
	err error
}

func (e *safeError) Error() string {
	if e == nil {
		return "secrets: operation failed"
	}
	if e.op == "" {
		return "secrets: operation failed"
	}
	return fmt.Sprintf("secrets: %s failed", e.op)
}

func (e *safeError) Unwrap() error { return e.err }

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return &safeError{op: op, err: err}
}

func opError(op string) error {
	return &safeError{op: op}
}
