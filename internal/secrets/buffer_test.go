// SPDX-License-Identifier: AGPL-3.0-or-later
package secrets

import "testing"

func TestBufferZeroizesOnClose(t *testing.T) {
	buf := NewBufferFromString("topsecret")
	if buf == nil {
		t.Fatalf("expected buffer")
	}
	raw := buf.Bytes()
	if len(raw) == 0 {
		t.Fatalf("expected buffer data")
	}
	buf.Close()
	for i, b := range raw {
		if b != 0 {
			t.Fatalf("expected zeroed byte at %d", i)
		}
	}
	if buf.Bytes() != nil {
		t.Fatalf("expected buffer released")
	}
}
