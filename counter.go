package hypertexting

import (
	"encoding/json"
	"strings"
	"sync"
)

type Counter struct {
	Counters map[string]int
	Lock     sync.RWMutex
}

func (c *Counter) Increment(value int, keys ...string) {
	if c.Counters == nil {
		c.Counters = map[string]int{}
	}
	for _, key := range keys {
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

func (c *Counter) Map() map[string]int {
	if c == nil {
		return nil
	}
	c.Lock.RLock()
	defer c.Lock.RUnlock()
	if len(c.Counters) == 0 {
		return nil
	}
	values := make(map[string]int, len(c.Counters))
	for key, value := range c.Counters {
		values[key] = value
	}
	return values
}

func (c Counter) MarshalJSON() ([]byte, error) {
	return json.Marshal((&c).Map())
}

func (c *Counter) UnmarshalJSON(data []byte) error {
	var values map[string]int
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	c.Counters = values
	c.Lock = sync.RWMutex{}
	return nil
}
