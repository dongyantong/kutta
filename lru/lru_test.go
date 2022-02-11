package kutta

import (
	"fmt"
	"testing"
	"time"
)

func TestLru(t *testing.T) {
	cache := New(2, time.Second*100)
	onEvicted := func(key Key, value interface{}) {
		fmt.Println(key, "is evicted ...")
	}
	cache.AddExWithOnEvicted("hello", "world", time.Second, &onEvicted)
	cache.Add("world", "hello")
	time.Sleep(time.Second * 5)
	hello, ok := cache.Get("hello")
	world, ok := cache.Get("world")
	fmt.Println(hello, ok)
	fmt.Println(world, ok)
}
