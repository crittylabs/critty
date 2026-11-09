// Package rendercache bounds generated previews by bytes and recency.
package rendercache

import "container/list"

const maxBytes = 16 << 20
const maxEntries = 256

// Cache is ready to use at its zero value. Callers serialize access and must
// not copy it after use.
type Cache[K comparable, V any] struct {
	items map[K]*list.Element
	order list.List
	bytes int
}

type entry[K comparable, V any] struct {
	key   K
	value V
	bytes int
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(entry[K, V]).value, true
	}
	var zero V
	return zero, false
}

func (c *Cache[K, V]) Put(key K, value V, bytes int) {
	if e, ok := c.items[key]; ok {
		c.remove(e)
	}
	if bytes > maxBytes {
		return
	}
	if c.items == nil {
		c.items = make(map[K]*list.Element)
	}
	c.items[key] = c.order.PushFront(entry[K, V]{key, value, bytes})
	c.bytes += bytes
	for c.bytes > maxBytes || len(c.items) > maxEntries {
		c.remove(c.order.Back())
	}
}

func (c *Cache[K, V]) remove(e *list.Element) {
	v := e.Value.(entry[K, V])
	delete(c.items, v.key)
	c.bytes -= v.bytes
	c.order.Remove(e)
}
