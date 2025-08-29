package hypertexting

import (
	"strings"
	"sync"
)

type Counter struct {
	Counters map[string]int
	Lock sync.RWMutex
}

func (c *Counter) Increment(value int, keys ...string) {
	if c.Counters == nil { c.Counters = map[string]int{} }
	for _,key := range keys {
		key = strings.ToLower(key)
		c.Lock.Lock()
		c.Counters[key] += value
		c.Lock.Unlock()
	}
}

func (c *Counter) Reset() {
	c.Counters = map[string]int{}
	c.Lock = sync.RWMutex{}
}
