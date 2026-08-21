// Package remediate 的 cooldown 机制：同事件同规则在冷却期内不重复执行。
package remediate

import (
	"sync"
	"time"
)

// Cooldown 记录 (eventID, ruleID) → 上次执行时间。
type Cooldown struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func NewCooldown() *Cooldown {
	return &Cooldown{last: make(map[string]time.Time)}
}

// InCooldown 判断是否在冷却期内。
func (c *Cooldown) InCooldown(eventID, ruleID string, cooldownSec int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := eventID + ":" + ruleID
	t, ok := c.last[key]
	if !ok {
		return false
	}
	return time.Since(t) < time.Duration(cooldownSec)*time.Second
}

// Mark 记录执行时间。
func (c *Cooldown) Mark(eventID, ruleID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last[eventID+":"+ruleID] = time.Now()
}
