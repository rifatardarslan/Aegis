package crypto

import (
	"runtime"
)

// Bytes explicitly overwrites the backing memory of the slice with zeroes.
// It uses runtime.KeepAlive to ensure the compiler does not optimize away the writes.
func Bytes(b []byte) {
	if b == nil {
		return
	}
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// Array32 overwrites a 32-byte array with zeroes.
func Array32(a *[32]byte) {
	if a == nil {
		return
	}
	for i := range a {
		a[i] = 0
	}
	runtime.KeepAlive(a)
}

// String releases this reference. Go strings cannot safely be overwritten;
// callers should use owned byte buffers for secret material.
func String(s *string) {
	if s != nil {
		*s = ""
	}
}
