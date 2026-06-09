package crypto

import (
	"runtime"
	"unsafe"
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

// String zeroes out the backing memory of a string directly.
// WARNING: This mutates the string's backing array which is technically immutable in Go.
// It should only be used for sensitive in-memory strings that are about to be discarded.
func String(s *string) {
	if s == nil || *s == "" {
		return
	}
	// Direct slice of the string's underlying backing array to mutate it in place
	hdr := unsafe.Slice(unsafe.StringData(*s), len(*s))
	for i := range hdr {
		hdr[i] = 0
	}
	runtime.KeepAlive(hdr)
	*s = ""
}

