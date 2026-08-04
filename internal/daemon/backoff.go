package daemon

import (
	"math/rand"
	"time"
)

// nextBackoff doubles the current interval, capped at max (section 15:
// "backoff exponencial").
func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max || next <= 0 { // overflow guard: next <= 0 means it wrapped
		return max
	}
	return next
}

// withJitter adds up to 20% random jitter on top of d (section 15:
// "backoff exponencial y jitter"), so a fleet of collectors retrying after
// the same outage doesn't hammer the backend in lockstep.
func withJitter(d time.Duration, rnd *rand.Rand) time.Duration {
	if d <= 0 {
		return d
	}
	jitter := time.Duration(rnd.Int63n(int64(d)/5 + 1))
	return d + jitter
}
