package living

import (
	"math"
	"testing"
)

// peak is how far a hop of the given velocity rises before gravity brings it
// back, which is what decides whether a step is cleared and how high the hop
// looks.
func peak(velocity, gravity float64) float64 {
	return velocity * velocity / (2 * gravity)
}

// Gravities spanning what entities get built with. A hop is handed over as a
// velocity, so these are what turn one velocity into very different heights:
// the walking units in Tower Wars now share 0.5, but they were spread from 0.07
// to 0.7 and anything built here can pick its own, so the derivation has to
// hold across the range rather than at one value.
var unitGravity = map[string]float64{
	"units (shared)": 0.5,
	"heavy":          0.7,
	"light":          0.08,
	"lightest":       0.07,
}

// The step a hop has to clear is the same for everyone: the pathfinder offers
// block-high steps to every unit and expects all of them to be able to take
// one.
const step = 1.0

func TestEveryUnitClearsTheStepItIsOffered(t *testing.T) {
	for name, gravity := range unitGravity {
		v := climbVelocity(gravity, step)
		if h := peak(v, gravity); h < step {
			t.Errorf("%s (gravity %v) hops %.2f blocks, short of the %v step the pathfinder offers it",
				name, gravity, h, step)
		}
	}
}

// A hop should clear the step and little more. The flat velocity of 1 this
// replaced threw the low gravity units over seven blocks up.
func TestNoUnitIsThrownFarAboveTheStep(t *testing.T) {
	const tolerance = 0.2

	for name, gravity := range unitGravity {
		v := climbVelocity(gravity, step)
		if h := peak(v, gravity); h > step+tolerance {
			t.Errorf("%s (gravity %v) hops %.2f blocks to clear a %v step", name, gravity, h, step)
		}
		if old := peak(1, gravity); old <= step+tolerance {
			continue
		}
		// Worth stating in the test output which units this actually changed.
		t.Logf("%s (gravity %v): %.2f blocks before, %.2f now", name, gravity, peak(1, gravity), peak(v, gravity))
	}
}

// 0.5 is the gravity the warrior had, the one unit whose hop already looked
// right, and it looked right because that gravity happens to suit a velocity of
// 1. It is what every unit is built with now. Deriving the velocity has to
// leave that case where it was, which is the check that the derivation agrees
// with what the game already does well rather than just being self-consistent.
func TestTheWarriorsHopIsUnchanged(t *testing.T) {
	v := climbVelocity(unitGravity["units (shared)"], step)
	if math.Abs(v-1) > 0.05 {
		t.Errorf("warrior hop velocity moved from 1 to %.3f", v)
	}
}

// Nothing brings a gravityless entity back down, so a hop derived from gravity
// is meaningless for one: it keeps what it asked for.
func TestNoGravityKeepsTheRequestedVelocity(t *testing.T) {
	if v := climbVelocity(0, step); v != step {
		t.Errorf("got %v, want %v", v, step)
	}
	if v := climbVelocity(-1, step); v != step {
		t.Errorf("negative gravity: got %v, want %v", v, step)
	}
}

func TestAHigherStepNeedsAFasterHop(t *testing.T) {
	g := 0.08
	if climbVelocity(g, 2) <= climbVelocity(g, 1) {
		t.Error("clearing a taller step should need more velocity")
	}
}
