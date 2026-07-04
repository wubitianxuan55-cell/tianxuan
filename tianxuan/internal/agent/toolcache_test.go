package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestToolCache_BasicGetSet(t *testing.T) {
	c := newToolCache(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	c.set(f, 0, "hello")
	val, ok := c.get(f, 0)
	if !ok {
		t.Fatal("cache miss after set")
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}
}

func TestToolCache_InvalidateOnWrite(t *testing.T) {
	c := newToolCache(-1)
	c.grace = 0 // disable grace optimisation so mtime invalidation is tested
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("v1"), 0644)

	c.set(f, 0, "v1")

	// 等待以确保文件 mtime 有可检测的变化（Windows FAT/NTFS 精度有限）
	time.Sleep(10 * time.Millisecond)

	// 修改文件
	os.WriteFile(f, []byte("v2"), 0644)

	_, ok := c.get(f, 0)
	if ok {
		t.Fatal("cache should be invalidated after file modification")
	}
}

func TestToolCache_TOCTOU_ConcurrentGetSet(t *testing.T) {
	// 并发 stress test：get 和 set 同时运行，验证缓存不会丢失有效条目。
	c := newToolCache(-1) // no TTL
	c.grace = 0           // disable grace so we exercise the Stat path in the race
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("initial"), 0644)

	c.set(f, 0, "initial")

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: 快速 get（触发 mtime 检查和可能的删除）
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.get(f, 0)
		}
	}()

	// Goroutine B: 快速 set 新内容
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.set(f, 0, fmt.Sprintf("v%d", i))
		}
	}()

	wg.Wait()

	// 最终缓存中应有有效条目（最后一次 set 的内容）
	val, ok := c.get(f, 0)
	if !ok {
		t.Fatal("缓存条目被 TOCTOU 竞态错误删除")
	}
	if val == "" {
		t.Fatal("缓存条目存在但内容为空")
	}
}

func TestToolCache_Clear(t *testing.T) {
	c := newToolCache(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("data"), 0644)

	c.set(f, 0, "data")
	c.clear()

	_, ok := c.get(f, 0)
	if ok {
		t.Fatal("cache should be empty after clear")
	}
}

func TestToolCache_InvalidatePath(t *testing.T) {
	c := newToolCache(-1)
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.txt")
	f2 := filepath.Join(tmp, "b.txt")
	os.WriteFile(f1, []byte("a"), 0644)
	os.WriteFile(f2, []byte("b"), 0644)

	c.set(f1, 0, "a")
	c.set(f2, 0, "b")
	c.invalidatePath(f1)

	_, ok := c.get(f1, 0)
	if ok {
		t.Fatal("f1 should be invalidated")
	}
	val, ok := c.get(f2, 0)
	if !ok || val != "b" {
		t.Fatal("f2 should still be cached")
	}
}

func TestToolCache_OffsetKeys(t *testing.T) {
	c := newToolCache(-1)
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("0123456789"), 0644)

	c.set(f, 0, "0123456789")
	c.set(f, 5, "56789")

	v0, ok0 := c.get(f, 0)
	v5, ok5 := c.get(f, 5)
	if !ok0 || v0 != "0123456789" {
		t.Fatal("offset 0 cache miss")
	}
	if !ok5 || v5 != "56789" {
		t.Fatal("offset 5 cache miss")
	}
}

func TestToolCache_GraceSkipsStat(t *testing.T) {
	// The grace period should return cached content without Stat for fresh entries.
	c := newToolCache(-1)
	c.grace = 5 * time.Second
	tmp := t.TempDir()
	f := filepath.Join(tmp, "f.txt")
	os.WriteFile(f, []byte("v1"), 0644)

	c.set(f, 0, "v1")

	// Modify file behind the cache's back — should still hit because within grace.
	time.Sleep(10 * time.Millisecond) // ensure mtime differs on low-precision FS
	os.WriteFile(f, []byte("v2"), 0644)

	val, ok := c.get(f, 0)
	if !ok {
		t.Fatal("cache miss within grace period")
	}
	if val != "v1" {
		t.Fatalf("expected cached 'v1', got %q", val)
	}

	// After grace expires, Stat should invalidate the stale entry.
	// Advance cached time to simulate age.
	c.mu.Lock()
	for k := range c.items {
		c.items[k].cached = time.Now().Add(-10 * time.Second)
	}
	c.mu.Unlock()

	_, ok = c.get(f, 0)
	if ok {
		t.Fatal("stale cache should be invalidated after grace")
	}
}
