package lru

import (
	"container/list"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

type Cache struct {
	MaxEntries int
	dl         *list.List
	cache      map[interface{}]*list.Element
	WatchDog   *watchDog
	lock       sync.RWMutex
}

type Key interface{}

type entry struct {
	key        Key
	value      interface{}
	Expiration int64
	OnEvicted  *func(key Key, value interface{})
}

func (e entry) Expired() bool {
	if e.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > e.Expiration
}

func New(maxEntries int, cleanupInterval time.Duration) *Cache {
	dog := &watchDog{
		Interval: cleanupInterval,
		stop:     make(chan bool),
	}
	c := &Cache{
		MaxEntries: maxEntries,
		dl:         list.New(),
		cache:      make(map[interface{}]*list.Element),
		WatchDog:   dog,
		lock:       sync.RWMutex{},
	}
	go dog.run(c)
	runtime.SetFinalizer(c, stopWatchDog)
	return c
}

func (c *Cache) Add(key Key, value interface{}) {
	c.add(key, value, -1, nil)
}

func (c *Cache) AddEx(key Key, value interface{}, d time.Duration) {
	c.add(key, value, d, nil)
}

func (c *Cache) AddExWithOnEvicted(key Key, value interface{}, d time.Duration, onEvicted *func(key Key, value interface{})) {
	c.add(key, value, d, onEvicted)
}

func (c *Cache) add(key Key, value interface{}, d time.Duration, onEvicted *func(key Key, value interface{})) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var e int64
	if c.cache == nil {
		c.cache = make(map[interface{}]*list.Element)
		c.dl = list.New()
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	if ee, ok := c.cache[key]; ok {
		c.dl.MoveToFront(ee)
		item := ee.Value.(*entry)
		item.value = value
		item.Expiration = e
		return
	}
	ele := c.dl.PushFront(&entry{key, value, e, onEvicted})
	c.cache[key] = ele
	if c.MaxEntries != 0 && c.dl.Len() > c.MaxEntries {
		c.RemoveOldest()
	}
}

func (c *Cache) Get(key Key) (value interface{}, ok bool) {
	if c.cache == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if ele, hit := c.cache[key]; hit {
		v := ele.Value.(*entry)
		if v.Expired() {
			c.removeElement(ele)
			// double check func evicted reload cache
			if ele, hit := c.cache[key]; hit {
				v := ele.Value.(*entry)
				return v.value, true
			}
			return
		}
		c.dl.MoveToFront(ele)
		return v.value, true
	}
	return
}

func (c *Cache) Remove(key Key) {
	if c.cache == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if ele, hit := c.cache[key]; hit {
		c.removeElement(ele)
	}
}

func (c *Cache) RemoveOldest() {
	if c.cache == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	ele := c.dl.Back()
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *Cache) removeElement(e *list.Element) {
	c.dl.Remove(e)
	kv := e.Value.(*entry)
	delete(c.cache, kv.key)
	if kv != nil && kv.OnEvicted != nil {
		onEvicted := *kv.OnEvicted
		onEvicted(kv.key, kv.value)
	}
}
func (c *Cache) DeleteExpired() {
	if c.Len() == 0 {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	now := time.Now().UnixNano()
	rand.Seed(now)
	count := rand.Intn(c.Len()) + 1
	for _, v := range c.cache {
		if count == 0 {
			return
		}
		count--
		kv := v.Value.(*entry)
		if kv.Expiration > 0 && now > kv.Expiration {
			c.removeElement(v)
		}
	}
}

func (c *Cache) Len() int {
	if c.cache == nil {
		return 0
	}
	return c.dl.Len()
}

func (c *Cache) Clear() {
	c.dl = nil
	c.cache = nil
}

type watchDog struct {
	Interval time.Duration
	stop     chan bool
}

func (dog *watchDog) run(c *Cache) {
	ticker := time.NewTicker(dog.Interval)
	for {
		select {
		case <-ticker.C:
			c.DeleteExpired()
		case <-dog.stop:
			ticker.Stop()
			return
		}
	}
}

func stopWatchDog(c *Cache) {
	c.WatchDog.stop <- true
}
