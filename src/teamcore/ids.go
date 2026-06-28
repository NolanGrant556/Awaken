package teamcore

import (
	"fmt"
	"sync/atomic"
	"time"
)

var teamcoreIDCounter uint64

func newID(prefix string) string {
	seq := atomic.AddUint64(&teamcoreIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
