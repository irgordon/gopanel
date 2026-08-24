package auth

import (
	"sync"
	"time"
)

const (
	maxAccountEntries = 1024
	accountAttempts   = 5
	accountWindow     = time.Minute
	globalBurst       = 20
	globalRefill      = time.Second
)

type accountLimit struct {
	count int
	reset time.Time
}
type LoginLimiter struct {
	mutex    sync.Mutex
	clock    func() time.Time
	accounts map[string]accountLimit
	tokens   float64
	last     time.Time
}

func NewLoginLimiter(clock func() time.Time) *LoginLimiter {
	now := clock()
	return &LoginLimiter{clock: clock, accounts: map[string]accountLimit{}, tokens: globalBurst, last: now}
}
func (limiter *LoginLimiter) Allow(email string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock()
	limiter.refill(now)
	if limiter.tokens < 1 {
		return false
	}
	limiter.tokens--
	limiter.evict(now)
	entry := limiter.accounts[email]
	if entry.reset.Before(now) {
		entry = accountLimit{reset: now.Add(accountWindow)}
	}
	if entry.count >= accountAttempts {
		return false
	}
	entry.count++
	limiter.accounts[email] = entry
	return true
}
func (limiter *LoginLimiter) Reset(email string) {
	limiter.mutex.Lock()
	delete(limiter.accounts, email)
	limiter.mutex.Unlock()
}
func (limiter *LoginLimiter) refill(now time.Time) {
	limiter.tokens += now.Sub(limiter.last).Seconds() / globalRefill.Seconds()
	if limiter.tokens > globalBurst {
		limiter.tokens = globalBurst
	}
	limiter.last = now
}
func (limiter *LoginLimiter) evict(now time.Time) {
	for key, value := range limiter.accounts {
		if value.reset.Before(now) {
			delete(limiter.accounts, key)
		}
	}
	if len(limiter.accounts) < maxAccountEntries {
		return
	}
	for key := range limiter.accounts {
		delete(limiter.accounts, key)
		if len(limiter.accounts) < maxAccountEntries {
			return
		}
	}
}
