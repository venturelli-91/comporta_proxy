// Package cache guarda respostas inteiras do provedor em memória, com TTL e limite em bytes.
package cache

import (
	"container/list"
	"sync"
	"time"
)

// Entrada é uma resposta pronta para repetir: o corpo JSON ou o stream SSE inteiro.
// Custo é o que a chamada original custou; cada hit registra isso como economia.
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

// Cache é um LRU limitado por bytes, não por quantidade: respostas de LLM vão de 100 bytes
// a centenas de KB, então contar itens não diz nada sobre a memória usada.
// ponytail: um mutex global; shards por hash da chave se a disputa aparecer no profile.
type Cache struct {
	mu       sync.Mutex
	max, uso int64
	ttl      time.Duration
	lista    *list.List // frente = mais recente
	itens    map[string]*list.Element
}

func Novo(maxBytes int64, ttl time.Duration) *Cache {
	return &Cache{max: maxBytes, ttl: ttl, lista: list.New(), itens: map[string]*list.Element{}}
}

// Get usa Lock (não RLock) porque um hit move o item para a frente da lista.
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
