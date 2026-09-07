//go:build darwin

package state

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID returns a 26 character ULID: 48 bits of millisecond time followed by
// 80 random bits, Crockford base32 encoded so ids sort by time.
func NewULID(now time.Time) string {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err)
	}
	var out [26]byte
	var acc uint64
	nbits := 0
	pos := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << nbits
		nbits += 8
		for nbits >= 5 {
			out[pos] = crockford[acc&31]
			acc >>= 5
			nbits -= 5
			pos--
		}
	}
	out[pos] = crockford[acc&31]
	return string(out[:])
}
