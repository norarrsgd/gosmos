// Package cache provides bounded, process-local document storage.
package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	key     string
	data    []byte
	expires time.Time
}
type Cache struct {
	version uint64
	mu      sync.Mutex
	entries map[string]*list.Element
	fifo    *list.List
	max     int
	ttl     time.Duration
	now     func() time.Time
}

func New(max int, ttl time.Duration) *Cache {
	return &Cache{entries: make(map[string]*list.Element), fifo: list.New(), max: max, ttl: ttl, now: time.Now}
}
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	v := e.Value.(entry)
	if !c.now().Before(v.expires) {
		c.remove(e)
		return nil, false
	}
	return append([]byte(nil), v.data...), true
}
func (c *Cache) Put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.put(key, data)
}
func (c *Cache) Version() uint64 { c.mu.Lock(); defer c.mu.Unlock(); return c.version }
func (c *Cache) PutIfVersion(key string, data []byte, version uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version == version {
		c.put(key, data)
	}
}
func (c *Cache) put(key string, data []byte) {
	if c.max <= 0 || c.ttl <= 0 {
		return
	}
	if e, ok := c.entries[key]; ok {
		c.remove(e)
	}
	for c.fifo.Len() >= c.max {
		c.remove(c.fifo.Front())
	}
	c.entries[key] = c.fifo.PushBack(entry{key, append([]byte(nil), data...), c.now().Add(c.ttl)})
}
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.remove(e)
	}
}
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.version++
	clear(c.entries)
	c.fifo.Init()
}
func (c *Cache) remove(e *list.Element) { delete(c.entries, e.Value.(entry).key); c.fifo.Remove(e) }
