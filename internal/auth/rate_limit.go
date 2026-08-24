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
func (limiter *LoginLimiter) AllowGlobal() bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock()
	limiter.refill(now)
	if limiter.tokens < 1 {
		return false
	}
	limiter.tokens--
	return true
}
func (limiter *LoginLimiter) AllowAccount(email string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.clock()
	limiter.evictExpired(now)
	entry, exists := limiter.accounts[email]
	if !exists && len(limiter.accounts) >= maxAccountEntries {
		return false
	}
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
func (limiter *LoginLimiter) evictExpired(now time.Time) {
	for key, value := range limiter.accounts {
		if value.reset.Before(now) {
			delete(limiter.accounts, key)
		}
	}
}
