package alerting

import "sync"

type ResolvedCache struct {
	mu     sync.Mutex
	alerts []Alert
	limit  int
}

func NewResolvedCache(limit int) *ResolvedCache {
	if limit <= 0 {
		limit = 12
	}
	return &ResolvedCache{limit: limit}
}
func (c *ResolvedCache) Add(alerts []Alert) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, alert := range alerts {
		if alert.Status.State != "resolved" {
			continue
		}
		found := false
		for i := range c.alerts {
			if alert.Fingerprint != "" && c.alerts[i].Fingerprint == alert.Fingerprint {
				c.alerts = append(c.alerts[:i], c.alerts[i+1:]...)
				found = true
				break
			}
		}
		_ = found
		c.alerts = append([]Alert{alert}, c.alerts...)
	}
	if len(c.alerts) > c.limit {
		c.alerts = c.alerts[:c.limit]
	}
}
func (c *ResolvedCache) List() []Alert {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Alert(nil), c.alerts...)
}
