// Package cache keeps whole provider responses in memory, with a TTL and a byte limit.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// Entrada (entry) is a response ready to replay: the JSON body or the whole SSE stream.
// Custo is what the original call cost; each hit records it as savings.
type Entrada struct {
	Corpo  []byte
	Stream bool
	Custo  int64
}

type item struct {
	chave  string
	e      Entrada
	expira time.Time
}

// Cache is an LRU bounded by bytes, not by count: LLM responses range from 100 bytes to
// hundreds of KB, so counting items says nothing about the memory used.
// ponytail: a global mutex; shards by key hash if contention shows up in a profile.
type Cache struct {
	mu       sync.Mutex
	max, uso int64
	ttl      time.Duration
	lista    *list.List // front = most recent
	itens    map[string]*list.Element
}

func Novo(maxBytes int64, ttl time.Duration) *Cache {
	return &Cache{max: maxBytes, ttl: ttl, lista: list.New(), itens: map[string]*list.Element{}}
}

// Get uses Lock (not RLock) because a hit moves the item to the front of the list.
func (c *Cache) Get(chave string) (Entrada, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.itens[chave]
	if !ok {
		return Entrada{}, false
	}
	it := el.Value.(*item)
	if time.Now().After(it.expira) {
		c.remover(el)
		return Entrada{}, false
	}
	c.lista.MoveToFront(el)
	return it.e, true
}

func (c *Cache) Set(chave string, e Entrada) {
	tam := int64(len(e.Corpo))
	if tam > c.max {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.itens[chave]; ok {
		c.remover(el)
	}
	for c.uso+tam > c.max {
		c.remover(c.lista.Back())
	}
	c.itens[chave] = c.lista.PushFront(&item{chave, e, time.Now().Add(c.ttl)})
	c.uso += tam
}

func (c *Cache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uso
}

func (c *Cache) remover(el *list.Element) {
	it := c.lista.Remove(el).(*item)
	delete(c.itens, it.chave)
	c.uso -= int64(len(it.e.Corpo))
}
