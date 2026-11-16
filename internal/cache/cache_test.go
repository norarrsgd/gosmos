package cache

import (
	"testing"
	"time"
)

func TestIsolationExpiryAndFIFO(t *testing.T) {
	c := New(2, time.Minute)
	now := time.Now()
	c.now = func() time.Time { return now }
	v := []byte("one")
	c.Put("a", v)
	v[0] = 'x'
	got, ok := c.Get("a")
	if !ok || string(got) != "one" {
		t.Fatal(string(got), ok)
	}
	got[0] = 'y'
	got, _ = c.Get("a")
	if string(got) != "one" {
		t.Fatal("shared read")
	}
	c.Put("b", []byte("two"))
	c.Put("c", []byte("three"))
	if _, ok = c.Get("a"); ok {
		t.Fatal("not evicted")
	}
	now = now.Add(time.Minute)
	if _, ok = c.Get("b"); ok {
		t.Fatal("not expired")
	}
	c.Clear()
	if _, ok = c.Get("c"); ok {
		t.Fatal("not cleared")
	}
}
func TestDisabled(t *testing.T) {
	c := New(0, time.Minute)
	c.Put("a", []byte("x"))
	if _, ok := c.Get("a"); ok {
		t.Fatal("disabled cache stored value")
	}
}
func TestInvalidationPreventsStaleFill(t *testing.T) {
	c := New(2, time.Minute)
	version := c.Version()
	c.Clear()
	c.PutIfVersion("a", []byte("stale"), version)
	if _, ok := c.Get("a"); ok {
		t.Fatal("stale in-flight response repopulated invalidated cache")
	}
}
