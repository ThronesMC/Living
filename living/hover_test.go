package living

import (
	"math"
	"testing"
)

// settle drifts a gap to nothing the way applyHover does, a tick at a time, and
// reports the path it took.
func settle(gap float64) []float64 {
	const giveUp = 400 // ticks, far past anything that should be needed

	path := []float64{gap}
	for range giveUp {
		step := hoverStep(gap)
		if step == 0 {
			return path
		}
		gap -= step
		path = append(path, gap)
	}
	return path
}

func TestHoverSettlesWithoutOvershooting(t *testing.T) {
	for _, gap := range []float64{0.05, 0.5, 1, 2, 5, -0.5, -1, -3, -8} {
		path := settle(gap)
		last := path[len(path)-1]

		if math.Abs(last) >= hoverSettled {
			t.Errorf("a gap of %v never settled: %d ticks in it is still %v", gap, len(path)-1, last)
			continue
		}
		// Crossing the height and coming back is a bounce, which is the
		// opposite of settling gently.
		for i, at := range path {
			if at*gap < 0 {
				t.Errorf("a gap of %v overshot to %v after %d ticks", gap, at, i)
				break
			}
		}
	}
}

// Easing means the drift slows as it arrives. Without that it would travel at
// its cap the whole way and stop dead on its height.
func TestHoverSlowsAsItArrives(t *testing.T) {
	path := settle(1)

	var steps []float64
	for i := 1; i < len(path); i++ {
		steps = append(steps, path[i-1]-path[i])
	}
	if len(steps) < 3 {
		t.Fatalf("a block of drift took only %d ticks, too few to ease", len(steps))
	}
	if first, last := steps[0], steps[len(steps)-1]; last >= first {
		t.Errorf("drift did not slow: started at %v a tick and ended at %v", first, last)
	}
}

// The caps are what stop a step up reading as a jump and a ledge as a drop, so
// a long way to travel goes at the cap rather than all at once.
func TestHoverRespectsItsCaps(t *testing.T) {
	if step := hoverStep(100); step != hoverRise {
		t.Errorf("rising from far below: got %v, want the %v cap", step, hoverRise)
	}
	if step := hoverStep(-100); step != -hoverFall {
		t.Errorf("sinking from far above: got %v, want the %v cap", step, -hoverFall)
	}
	if hoverFall <= hoverRise {
		t.Errorf("coming down (%v) should be no slower than going up (%v)", hoverFall, hoverRise)
	}
}

// Graceful is a pace, not just a direction: a block should take long enough to
// read as a float and not so long that the unit is visibly lagging the ground.
func TestHoverTakesAGracefulTimeOverABlock(t *testing.T) {
	const (
		tick    = 0.05 // seconds
		fastest = 0.3  // seconds, below which it is a jump rather than a float
		slowest = 2.0  // seconds, past which it is trailing the ground
	)

	for _, gap := range []float64{1, -1} {
		took := float64(len(settle(gap))-1) * tick
		if took < fastest || took > slowest {
			t.Errorf("a %v block move took %.2fs, outside %v-%vs", gap, took, fastest, slowest)
		}
	}
}

func TestHoverHoldsStillOnceSettled(t *testing.T) {
	if step := hoverStep(0); step != 0 {
		t.Errorf("at its height it should hold, got %v", step)
	}
	if step := hoverStep(hoverSettled / 2); step != 0 {
		t.Errorf("within the settle threshold it should hold, got %v", step)
	}
}
