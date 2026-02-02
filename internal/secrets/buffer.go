// SPDX-License-Identifier: AGPL-3.0-or-later
package secrets

// Buffer holds secret bytes that can be explicitly zeroed on cleanup.
// Call Close() to overwrite the underlying bytes.
type Buffer struct {
	data []byte
}

// NewBufferFromString copies s into a new zeroizable buffer.
func NewBufferFromString(s string) *Buffer {
	if s == "" {
		return &Buffer{data: nil}
	}
	buf := make([]byte, len(s))
	copy(buf, s)
	return &Buffer{data: buf}
}

// Bytes returns the underlying secret bytes.
func (b *Buffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data
}

// Len returns the length of the underlying secret bytes.
func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}
	return len(b.data)
}

// Close zeroes the underlying bytes and releases the buffer.
func (b *Buffer) Close() {
	if b == nil {
		return
	}
	for i := range b.data {
		b.data[i] = 0
	}
	b.data = nil
}
