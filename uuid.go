package finlight

import (
	"crypto/rand"
	"fmt"
)

// newUUIDv4 returns a random RFC 4122 version-4 UUID. Used as the WebSocket
// handshake nonce; not worth an external dependency.
func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("finlight: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
