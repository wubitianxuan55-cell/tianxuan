package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBasicGetSet(t *testing.T) {
	c := New(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	c.Set(f, 0, "hello")
	val, ok := c.Get(f, 0)
	if !ok {
		t.Fatal("cache miss after set")
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
}

func TestInvalidateOnWrite(t *testing.T) {
	c := New(-1)
	c.grace = 0
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("v1"), 0644)

	c.Set(f, 0, "v1")
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(f, []byte("v2"), 0644)

	_, ok := c.Get(f, 0)
	if ok {
		t.Fatal("cache should be invalidated after file modification")
	}
}

func TestTOCTOUConcurrentGetSet(t *testing.T) {
	c := New(-1)
	c.grace = 0
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("initial"), 0644)

	c.Set(f, 0, "initial")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.Get(f, 0)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.Set(f, 0, fmt.Sprintf("v%d", i))
		}
	}()

	wg.Wait()

	val, ok := c.Get(f, 0)
	if !ok {
		t.Fatal("缓存条目被 TOCTOU 竞态错误删除")
	}
	if val == "" {
		t.Fatal("缓存条目存在但内容为空")
	}
}

func TestClear(t *testing.T) {
	c := New(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("data"), 0644)

	c.Set(f, 0, "data")
	c.Clear()

	_, ok := c.Get(f, 0)
	if ok {
		t.Fatal("cache should be empty after clear")
	}
}

func TestInvalidatePath(t *testing.T) {
	c := New(-1)
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.txt")
	f2 := filepath.Join(tmp, "b.txt")
	os.WriteFile(f1, []byte("a"), 0644)
	os.WriteFile(f2, []byte("b"), 0644)

	c.Set(f1, 0, "a")
	c.Set(f2, 0, "b")
	c.InvalidatePath(f1)

	_, ok := c.Get(f1, 0)
	if ok {
		t.Fatal("f1 should be invalidated")
	}
	val, ok := c.Get(f2, 0)
	if !ok || val != "b" {
		t.Fatal("f2 should still be cached")
	}
}

func TestOffsetKeys(t *testing.T) {
	c := New(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("0123456789"), 0644)

	c.Set(f, 0, "0123456789")
	c.Set(f, 5, "56789")

	v0, ok0 := c.Get(f, 0)
	v5, ok5 := c.Get(f, 5)
	if !ok0 || v0 != "0123456789" {
		t.Fatal("offset 0 cache miss")
	}
	if !ok5 || v5 != "56789" {
		t.Fatal("offset 5 cache miss")
	}
}

func TestGraceSkipsStat(t *testing.T) {
	c := New(-1)
	c.grace = 5 * time.Second
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("v1"), 0644)

	c.Set(f, 0, "v1")

	time.Sleep(10 * time.Millisecond)
	os.WriteFile(f, []byte("v2"), 0644)

	val, ok := c.Get(f, 0)
	if !ok {
		t.Fatal("cache miss within grace period")
	}
	if val != "v1" {
		t.Fatalf("expected cached 'v1', got %q", val)
	}

	// After grace expires, Stat should invalidate the stale entry.
	c.mu.Lock()
	for k := range c.items {
		c.items[k].cached = time.Now().Add(-10 * time.Second)
	}
	c.mu.Unlock()

	_, ok = c.Get(f, 0)
	if ok {
		t.Fatal("stale cache should be invalidated after grace")
	}
}
