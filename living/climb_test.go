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

// The gravity each unit in Tower Wars is built with. A hop is handed over as a
// velocity, so these are what turn one velocity into very different heights.
var unitGravity = map[string]float64{
	"warrior":      0.5,
	"bowler":       0.5,
	"mage":         0.7,
	"villager":     0.08,
	"berserk":      0.08,
	"executioner":  0.08,
	"giant beast":  0.08,
	"assassin":     0.07,
	"babySkeleton": 0.07,
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

// The warrior is the one unit whose hop already looked right, and it is right
// because its gravity happens to suit a velocity of 1. Deriving the velocity
// has to leave it where it was, which is the check that the derivation matches
// what the game already does well rather than just being self-consistent.
func TestTheWarriorsHopIsUnchanged(t *testing.T) {
	v := climbVelocity(unitGravity["warrior"], step)
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
