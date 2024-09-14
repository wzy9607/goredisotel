//go:build !appengine
// +build !appengine

package rediscmd

import "unsafe"

// String converts byte slice to string.
//
//nolint:gosec // done
func String(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// Bytes converts string to byte slice.
//
//nolint:gosec // done
func Bytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}
