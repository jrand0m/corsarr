package httpx

import "time"

// countWindows are the sliding windows every service's traffic is counted on.
var countWindows = []time.Duration{time.Second, time.Minute, time.Hour}

const slotsPerWindow = 60

// slidingCount counts events over the last window in fixed slots, so memory
// stays constant however busy the service is; the count is exact to one slot.
type slidingCount struct {
	slot  time.Duration
	slots [slotsPerWindow]int32
	epoch int64 // index of the newest slot
}

func newSliding(window time.Duration) *slidingCount {
	return &slidingCount{slot: window / slotsPerWindow}
}

// advance moves the newest slot to now, emptying slots that fell out.
func (s *slidingCount) advance(now time.Time) {
	idx := now.UnixNano() / int64(s.slot)
	if idx <= s.epoch {
		return
	}
	for i := s.epoch + 1; i <= idx && i <= s.epoch+slotsPerWindow; i++ {
		s.slots[i%slotsPerWindow] = 0
	}
	s.epoch = idx
}

func (s *slidingCount) add(now time.Time) {
	s.advance(now)
	s.slots[s.epoch%slotsPerWindow]++
}

func (s *slidingCount) count(now time.Time) int {
	s.advance(now)
	n := 0
	for _, v := range s.slots {
		n += int(v)
	}
	return n
}

// windowFor is the shortest counting window that covers d.
func windowFor(d time.Duration) time.Duration {
	for _, w := range countWindows {
		if d <= w {
			return w
		}
	}
	return countWindows[len(countWindows)-1]
}
