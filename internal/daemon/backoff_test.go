package daemon

import (
	"math/rand"
	"testing"
	"time"
)

func TestNextBackoffDoubles(t *testing.T) {
	got := nextBackoff(10*time.Second, time.Hour)
	if got != 20*time.Second {
		t.Errorf("nextBackoff() = %v, want 20s", got)
	}
}

func TestNextBackoffCapsAtMax(t *testing.T) {
	got := nextBackoff(50*time.Minute, time.Hour)
	if got != time.Hour {
		t.Errorf("nextBackoff() = %v, want capped at 1h", got)
	}
}

func TestNextBackoffMonotonicUntilCap(t *testing.T) {
	max := 10 * time.Minute
	interval := 1 * time.Second
	for i := 0; i < 20; i++ {
		next := nextBackoff(interval, max)
		if next < interval {
			t.Fatalf("backoff decreased: %v -> %v", interval, next)
		}
		if next > max {
			t.Fatalf("backoff exceeded max: %v > %v", next, max)
		}
		interval = next
	}
	if interval != max {
		t.Errorf("interval = %v after many iterations, want converged to max %v", interval, max)
	}
}

func TestWithJitterAddsUpToTwentyPercent(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	base := 100 * time.Second
	for i := 0; i < 100; i++ {
		got := withJitter(base, rnd)
		if got < base {
			t.Fatalf("withJitter() = %v, want >= base %v", got, base)
		}
		if got > base+base/5+time.Second {
			t.Fatalf("withJitter() = %v, want <= ~120%% of base", got)
		}
	}
}

func TestWithJitterZeroIsZero(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	if got := withJitter(0, rnd); got != 0 {
		t.Errorf("withJitter(0) = %v, want 0", got)
	}
}
