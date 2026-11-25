package finlight

import (
	"context"
	"time"
)

// Clock is the time source of the SDK: retry and burst waits, stream pings,
// the watchdog, rotation and reconnect delays. Tests replace it with a fake;
// the default is the system clock.
type Clock interface {
	Now() time.Time
	// NewTimer returns a timer that fires once after d.
	NewTimer(d time.Duration) Timer
	// NewTicker returns a ticker that fires every d.
	NewTicker(d time.Duration) Timer
}

// Timer is a timer or ticker created by a Clock.
type Timer interface {
	C() <-chan time.Time
	// Stop stops the timer; it does not close the channel.
	Stop()
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTimer(d time.Duration) Timer { return sysTimer{t: time.NewTimer(d)} }

func (systemClock) NewTicker(d time.Duration) Timer { return sysTicker{t: time.NewTicker(d)} }

type sysTimer struct{ t *time.Timer }

func (s sysTimer) C() <-chan time.Time { return s.t.C }
func (s sysTimer) Stop()               { s.t.Stop() }

type sysTicker struct{ t *time.Ticker }

func (s sysTicker) C() <-chan time.Time { return s.t.C }
func (s sysTicker) Stop()               { s.t.Stop() }

// sleep waits d on clock, or returns ctx.Err() as soon as ctx is done.
func sleep(ctx context.Context, clock Clock, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := clock.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C():
		return nil
	}
}
