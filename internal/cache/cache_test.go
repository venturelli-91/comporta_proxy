package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func entrada(n int) Entrada { return Entrada{Corpo: make([]byte, n)} }

func TestGetSet(t *testing.T) {
	c := Novo(1000, time.Minute)
	if _, ok := c.Get("x"); ok {
		t.Fatal("empty cache returned a hit")
	}
	c.Set("x", Entrada{Corpo: []byte("oi"), Stream: true, Custo: 7})
	e, ok := c.Get("x")
	if !ok || string(e.Corpo) != "oi" || !e.Stream || e.Custo != 7 {
		t.Fatalf("Get = %+v, %v", e, ok)
	}
}

func TestExpiraPorTTL(t *testing.T) {
	c := Novo(1000, 10*time.Millisecond)
	c.Set("x", entrada(1))
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("x"); ok {
		t.Fatal("expired entry returned a hit")
	}
	if c.Bytes() != 0 {
		t.Fatalf("Bytes = %d after expiring, want 0", c.Bytes())
	}
}

func TestDespejaOMenosUsadoPorBytes(t *testing.T) {
	c := Novo(300, time.Minute)
	c.Set("a", entrada(100))
	c.Set("b", entrada(100))
	c.Set("c", entrada(100))
	c.Get("a") // "a" becomes the most recent; "b" becomes the candidate
	c.Set("d", entrada(100))
	if _, ok := c.Get("b"); ok {
		t.Fatal(`"b" was the least recently used and should have been evicted`)
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Errorf("%q should still be in the cache", k)
		}
	}
	if c.Bytes() > 300 {
		t.Fatalf("Bytes = %d above the limit", c.Bytes())
	}
}

func TestEntradaMaiorQueOLimiteNaoEntra(t *testing.T) {
	c := Novo(100, time.Minute)
	c.Set("grande", entrada(101))
	if _, ok := c.Get("grande"); ok || c.Bytes() != 0 {
		t.Fatal("an entry larger than the whole cache should not be stored")
	}
}

func TestSubstituirAtualizaBytes(t *testing.T) {
	c := Novo(1000, time.Minute)
	c.Set("x", entrada(100))
	c.Set("x", entrada(10))
	if c.Bytes() != 10 {
		t.Fatalf("Bytes = %d, want 10", c.Bytes())
	}
}

func TestConcorrente(t *testing.T) {
	c := Novo(10_000, time.Minute)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for j := range 100 {
				k := fmt.Sprint((i + j) % 30)
				c.Set(k, entrada(50))
				c.Get(k)
			}
		})
	}
	wg.Wait()
	if c.Bytes() > 10_000 {
		t.Fatalf("Bytes = %d above the limit", c.Bytes())
	}
}
