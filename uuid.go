package gosmos

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

var uuidState struct {
	sync.Mutex
	millis   int64
	sequence uint16
}

func newUUIDv7() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	uuidState.Lock()
	now := time.Now().UnixMilli()
	if now > uuidState.millis {
		uuidState.millis = now
		uuidState.sequence = 0
	} else {
		uuidState.sequence++
		if uuidState.sequence > 4095 {
			uuidState.millis++
			uuidState.sequence = 0
		}
	}
	binary.BigEndian.PutUint64(b[:8], uint64(uuidState.millis)<<16|uint64(0x7000|uuidState.sequence))
	uuidState.Unlock()
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
