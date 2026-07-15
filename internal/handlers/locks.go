package handlers

import (
	"hash/fnv"
	"sync"
)

// sessionLocks serializes callback handling per session id. Striped instead
// of per-key so there is nothing to clean up; 64 stripes is plenty for one
// bot instance (single-instance deployment is a design assumption).
type sessionLocks struct {
	stripes [64]sync.Mutex
}

// lock acquires the stripe for key and returns its unlock func.
func (l *sessionLocks) lock(key string) func() {
	h := fnv.New32a()
	h.Write([]byte(key))
	m := &l.stripes[h.Sum32()%uint32(len(l.stripes))]
	m.Lock()
	return m.Unlock
}
