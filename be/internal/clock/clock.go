package clock

import "time"

// Clock provides time operations, allowing test code to control time.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

// Real returns a Clock that uses the system clock.
func Real() Clock { return realClock{} }

func (realClock) Now() time.Time { return time.Now() }
