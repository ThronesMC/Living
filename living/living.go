package living

import (
	"iter"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/event"
	"github.com/df-mc/dragonfly/server/item/enchantment"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/sound"
	"github.com/go-gl/mathgl/mgl64"
)

var _ = world.Entity(&Living{})
var _ = entity.Living(&Living{})

type Living struct {
	handle *world.EntityHandle
	tx     *world.Tx
	data   *world.EntityData

	*livingData
}

func (l *Living) Heal(health float64, _ world.HealingSource) float64 {
	if l.Dead() || health < 0 {
		return 0
	}
	old := l.Health()
	l.AddHealth(health)
	return l.Health() - old
}

func (l *Living) Hurt(dmg float64, src world.DamageSource) (float64, bool) {
	if l.Dead() || dmg <= 0 {
		return 0, false
	}
	totalDamage := dmg
	damageLeft := totalDamage

	immune := time.Now().Before(l.immuneUntil)
	if immune {
		if damageLeft = damageLeft - l.lastDamage; damageLeft <= 0 {
			return 0, false
		}
	}

	immunity := l.immuneDuration
	ctx := event.C[*Living](l)
	if l.handler.HandleHurt(*ctx, totalDamage, immune, &immunity, src); ctx.Cancelled() {
		return 0, false
	}
	l.setAttackImmunity(immunity, totalDamage)
	l.AddHealth(-damageLeft)

	pos := l.Position()
	for _, viewer := range l.Viewers() {
		viewer.ViewEntityAction(l, entity.HurtAction{})
	}
	if src.Fire() {
		l.tx.PlaySound(pos, sound.Burning{})
	} else if _, ok := src.(entity.DrowningDamageSource); ok {
		l.tx.PlaySound(pos, sound.Drowning{})
	}

	if l.Dead() {
		l.Kill(src)
	}
	return totalDamage, true
}

func (l *Living) Kill(_ world.DamageSource) {
	for _, viewer := range l.Viewers() {
		viewer.ViewEntityAction(l, entity.DeathAction{})
	}

	l.AddHealth(-l.MaxHealth())
	l.DropItems()

	// Wait a little before removing the entity. The client displays a death
	// animation while the player is dying.
	time.AfterFunc(time.Millisecond*1100, func() {
		l.H().Do(finishDying)
	})
}

// finishDying completes the death of a player, removing it from the world.
func finishDying(_ *world.Tx, e world.Entity) {
	p := e.(entity.Living)
	_ = p.Close()
}

func (l *Living) DropItems() {
	pos := l.Position()
	for d := range l.drops {
		it := d.Stack()
		if it.Empty() {
			continue
		}
		if _, ok := it.Enchantment(enchantment.CurseOfVanishing); ok {
			continue
		}
		opts := world.EntitySpawnOpts{Position: pos}
		l.tx.AddEntity(entity.NewItem(opts, it))
	}
}

// setAttackImmunity sets the duration the player is immune to entity attacks.
func (l *Living) setAttackImmunity(d time.Duration, dmg float64) {
	l.immuneUntil = time.Now().Add(d)
	l.lastDamage = dmg
}

// KnockBack knocks the player back with a given force and height. A source is passed which indicates the
// source of the velocity, typically the position of an attacking entity. The source is used to calculate the
// direction which the entity should be knocked back in.
func (l *Living) KnockBack(src mgl64.Vec3, force, height float64) {
	if l.Dead() {
		return
	}
	l.knockBack(src, force, height)
}

// knockBack is an unexported function that is used to knock the player back. This function does not check if the player
// can take damage or not.
func (l *Living) knockBack(src mgl64.Vec3, force, height float64) {
	velocity := l.Position().Sub(src)
	velocity[1] = 0

	if velocity.Len() != 0 {
		velocity = velocity.Normalize().Mul(force)
	}
	velocity[1] = height

	l.SetVelocity(velocity.Mul(1))
}

// Tx returns the transaction.
func (l *Living) Tx() *world.Tx {
	return l.tx
}

// Age returns the age of the entity.
func (l *Living) Age() time.Duration {
	return l.age
}

// Speed returns the speed.
func (l *Living) Speed() float64 {
	return l.speed
}

// SetSpeed sets the speed.
func (l *Living) SetSpeed(f float64) {
	l.speed = f
}

// Velocity returns the velocity.
func (l *Living) Velocity() mgl64.Vec3 {
	return l.data.Vel
}

// SetVelocity sets the velocity.
func (l *Living) SetVelocity(velocity mgl64.Vec3) {
	l.data.Vel = velocity
	for _, v := range l.Viewers() {
		v.ViewEntityVelocity(l, velocity)
	}
}

// Drops returns the drops.
func (l *Living) Drops() iter.Seq[Drop] {
	return l.drops
}

// AddEffect adds an effect to the entity.
func (l *Living) AddEffect(e effect.Effect) {
	l.effects[e.Type()] = e
}

// RemoveEffect removes the effect of an entity.
func (l *Living) RemoveEffect(e effect.Type) {
	delete(l.effects, e)
}

// Effects returns the effects of an entity.
func (l *Living) Effects() []effect.Effect {
	return slices.Collect(maps.Values(l.effects))
}

// Close closes the entity.
func (l *Living) Close() error {
	l.tx.RemoveEntity(l)
	return nil
}

// H returns the EntityHandle.
func (l *Living) H() *world.EntityHandle {
	return l.handle
}

// Position returns the position.
func (l *Living) Position() mgl64.Vec3 {
	return l.data.Pos
}

// Rotation returns the rotation.
func (l *Living) Rotation() cube.Rotation {
	return l.data.Rot
}

// SetRotation sets the rotation.
func (l *Living) SetRotation(yaw, pitch float64) {
	currentRotation := l.Rotation()

	deltaYaw := yaw - currentRotation.Yaw()
	deltaPitch := pitch - currentRotation.Pitch()

	l.Move(mgl64.Vec3{}, deltaYaw, deltaPitch)
}

// Dead returns if the entity is dead or not.
func (l *Living) Dead() bool {
	return l.Health() <= mgl64.Epsilon
}

// OnGround returns if the entity is on the ground.
func (l *Living) OnGround() bool {
	return l.onGround
}

// Immobile returns if the entity is Immobile.
func (l *Living) Immobile() bool {
	return l.immobile
}

// SetImmobile prevents the living from moving around, but still allows them to look around.
func (l *Living) SetImmobile() {
	if l.immobile {
		return
	}
	l.immobile = true
	l.updateState()
}

// SetMobile allows the living to freely move around again after being immobile.
func (l *Living) SetMobile() {
	if !l.Immobile() {
		return
	}
	l.immobile = false
	l.updateState()
}

// Invisible ...
func (l *Living) Invisible() bool {
	return l.invisible
}

// SetInvisible ...
func (l *Living) SetInvisible(invisible bool) {
	l.invisible = invisible
	l.updateState()
}

// Scale ...
func (l *Living) Scale() float64 {
	return l.scale
}

// SetScale ...
func (l *Living) SetScale(scale float64) {
	l.scale = scale
	l.updateState()
}

// EyeHeight ...
func (l *Living) EyeHeight() float64 {
	return l.livingData.eyeHeight
}

// NameTag ...
func (l *Living) NameTag() string {
	return l.data.Name
}

// SetNameTag ...
func (l *Living) SetNameTag(s string) {
	l.data.Name = s
	l.updateState()
}

// Move moves the player from one position to another in the world, by adding the delta passed to the current
// position of the player.
// Move also rotates the player, adding deltaYaw and deltaPitch to the respective values.
func (l *Living) Move(deltaPos mgl64.Vec3, deltaYaw, deltaPitch float64) {
	if l.Dead() || (deltaPos.ApproxEqual(mgl64.Vec3{}) && mgl64.FloatEqual(deltaYaw, 0) && mgl64.FloatEqual(deltaPitch, 0)) {
		return
	}
	if l.immobile {
		if mgl64.FloatEqual(deltaYaw, 0) && mgl64.FloatEqual(deltaPitch, 0) {
			// If only the position was changed, don't continue with the movement when Immobile.
			return
		}
		// Still update rotation if it was changed.
		deltaPos = mgl64.Vec3{}
	}
	var (
		pos        = l.Position()
		yaw, pitch = l.Rotation().Elem()
		resRot     = cube.Rotation{yaw + deltaYaw, pitch + deltaPitch}
	)

	// Check collisions BEFORE updating position
	originalDelta := deltaPos
	if deltaPos.Len() <= 3 {
		// Apply collision detection to modify deltaPos
		deltaPos = l.calculateCollisionAdjustedMovement(deltaPos)
		l.data.Vel = originalDelta
	}

	// Now calculate final position with collision-adjusted deltaPos
	res := pos.Add(deltaPos)

	for _, v := range l.Viewers() {
		v.ViewEntityMovement(l, res, resRot, l.OnGround())
	}

	l.data.Pos = res
	l.data.Rot = resRot

	l.onGround = l.checkOnGround()
	l.updateFallState(deltaPos[1])
}

// climbClearance is how far above a step an entity's hop reaches, so that it
// comes down onto the step rather than exactly level with its edge.
const climbClearance = 0.05

// climbVelocity returns the upward velocity a hop needs to reach climb blocks
// above the entity's feet.
//
// The velocity a hop needs depends on the entity's gravity, which is set per
// entity: the hop is handed to the movement computer as a velocity and gravity
// is what brings it back down, so the same velocity on a lighter entity throws
// it far higher. Every entity was given a velocity of 1, which on the 0.5
// gravity a warrior has peaks at about the one block it is meant to clear, and
// on the 0.07 a baby skeleton has peaks above seven.
//
// Deriving it from gravity instead has every entity clear the same step, which
// is what the pathfinder assumes when it offers one. An entity with no gravity
// never comes down on its own, so it keeps the velocity it was asked for.
func climbVelocity(gravity, climb float64) float64 {
	if gravity <= 0 {
		return climb
	}
	return math.Sqrt(2 * gravity * (climb + climbClearance))
}

// MoveToTarget steps the entity towards target, turning to face it and climbing
// a step if one is in the way. maxClimb is the tallest step it can take.
//
// The step and the turn are applied in a single Move, because every Move sends
// a movement update to viewers and the client drives its walking animation from
// how far the entity travelled between updates. A turn sent as its own update
// carries no distance, so it reads as the entity having stopped, and the
// animation stops with it.
func (l *Living) MoveToTarget(target mgl64.Vec3, maxClimb float64) {
	if l.Dead() {
		return
	}

	delta := target.Sub(l.Position())
	delta[1] = 0
	if delta.Len() == 0 {
		return
	}
	dir := delta.Normalize()
	move := dir.Mul(l.Speed())

	// Look one body width ahead for something to climb.
	checkPos := cube.PosFromVec3(l.Position().Add(dir.Mul(l.H().Type().BBox(l).Width())))
	low := blockTop(l.tx, checkPos)
	high := blockTop(l.tx, checkPos.Add(cube.Pos{0, 1, 0}))

	// How far above the entity's feet the obstacle reaches.
	rise := float64(checkPos.Y()) + low - l.Position().Y()

	climbable := high <= maxClimb && low+high <= maxClimb
	if climbable && rise > 0.01 && rise <= maxClimb && l.OnGround() {
		// Hop up, easing off horizontally while leaving the ground.
		move[1] = climbVelocity(l.mc.Gravity, maxClimb)
		move[0] *= 0.50
		move[2] *= 0.50
	}

	if !l.OnGround() {
		move[0] *= 0.25
		move[2] *= 0.25
	}

	// Anything taller than a step is left to collision resolution, which
	// clamps each axis separately and so slides the entity along the
	// obstruction. Cancelling the whole horizontal step here instead, as this
	// used to, stopped an entity dead the moment anything tall was ahead of
	// it - including while it was walking *past* a wall rather than into it,
	// which left units stuck against corners they were trying to round.

	yaw, pitch := LookAtExtended(l.Position().Add(mgl64.Vec3{0, l.EyeHeight(), 0}), target)
	l.Move(move, yaw-l.Rotation().Yaw(), pitch-l.Rotation().Pitch())
}

// blockTop returns how far up the block at pos reaches, measured from the
// block's own base, or zero if it does not obstruct.
//
// The real position is handed to BBox rather than a zero one: models such as
// fences, walls and panes inspect their neighbours through it, so a zero
// position describes the shape of some other block entirely.
func blockTop(tx *world.Tx, pos cube.Pos) float64 {
	top := 0.0
	for _, box := range tx.Block(pos).Model().BBox(pos, tx) {
		if h := box.Max()[1]; h > top {
			top = h
		}
	}
	return top
}

// LookAt ...
func (l *Living) LookAt(v mgl64.Vec3) {
	yaw, pitch := LookAtExtended(l.Position().Add(mgl64.Vec3{0, l.EyeHeight(), 0}), v)
	dy := yaw - l.Rotation().Yaw()
	dp := pitch - l.Rotation().Pitch()

	l.Move(mgl64.Vec3{0, 0, 0}, dy, dp)
}

// LookAwayFrom ...
func (l *Living) LookAwayFrom(v mgl64.Vec3) {
	yaw, pitch := LookAtExtended(l.Position().Add(mgl64.Vec3{0, l.EyeHeight(), 0}), v)
	dy := int(math.Round(yaw - l.Rotation().Yaw()))
	dp := pitch - l.Rotation().Pitch()

	dy = (dy + 180) % 360
	if dy > 180 {
		dy -= 360
	}

	l.Move(mgl64.Vec3{0, 0, 0}, float64(dy), -dp)
}

// LookAtExtended ...
func LookAtExtended(pos mgl64.Vec3, v mgl64.Vec3) (yaw float64, pitch float64) {
	vt := v.Y() - pos.Y()
	hz := math.Sqrt(math.Pow(v.X()-pos.X(), 2) + math.Pow(v.Z()-pos.Z(), 2))
	pitch = (-math.Atan2(vt, hz) / math.Pi) * 180

	dz := v.Z() - pos.Z()
	dx := v.X() - pos.X()
	yaw = (math.Atan2(dz, dx)/math.Pi)*180 - 90
	if yaw < 0 {
		yaw += 360.0
	}

	return yaw, pitch
}

// ResetFallDistance resets the player's fall distance.
func (l *Living) ResetFallDistance() {
	l.fallDistance = 0
}

// FallDistance returns the player's fall distance.
func (l *Living) FallDistance() float64 {
	return l.fallDistance
}

// OnFireDuration ...
func (l *Living) OnFireDuration() time.Duration {
	return time.Duration(l.fireTicks) * time.Second / 20
}

// SetOnFire ...
func (l *Living) SetOnFire(duration time.Duration) {
	ticks := int64(duration.Seconds() * 20)
	l.fireTicks = ticks
	l.updateState()
}

// Extinguish ...
func (l *Living) Extinguish() {
	l.SetOnFire(0)
}

// ImmuneUntil ...
func (l *Living) ImmuneUntil() time.Time {
	return l.immuneUntil
}

// SetImmuneDuration ...
func (l *Living) SetImmuneDuration(duration time.Duration) {
	l.immuneDuration = duration
}

// ImmuneDuration ...
func (l *Living) ImmuneDuration() time.Duration {
	return l.immuneDuration
}

// AttackImmune ...
func (l *Living) AttackImmune() bool {
	return l.ImmuneUntil().After(time.Now())
}

// LastDamage ...
func (l *Living) LastDamage() float64 {
	return l.lastDamage
}

// Tick ticks the entity, performing actions such as checking if the player is still breaking a block.
func (l *Living) Tick(tx *world.Tx, current int64) {
	l.age += 50 * time.Millisecond
	ctx := event.C(l)
	l.handler.HandleTick(*ctx, tx)

	if ctx.Cancelled() || l.Dead() {
		return
	}

	if l.Position()[1] < float64(tx.Range()[0]) && current%10 == 0 {
		l.Hurt(4, entity.VoidDamageSource{})
	}

	if l.OnFireDuration() > 0 {
		l.fireTicks -= 1
		if l.OnFireDuration() <= 0 || tx.RainingAt(cube.PosFromVec3(l.Position())) {
			l.Extinguish()
		}
		if l.OnFireDuration()%time.Second == 0 {
			l.Hurt(1, block.FireDamageSource{})
		}
	}

	l.onGround = l.checkOnGround()

	m := l.mc.TickMovement(l, l.Position(), l.Velocity(), l.Rotation(), tx)
	m.Send()

	// Commit what the movement computer worked out, without going through Move.
	// Send has already told viewers the new position, and told them the new
	// velocity, which is what lets a client tell that the entity is moving at
	// all. Move would send the same position a second time, as an update with
	// no distance in it, and would resolve collisions a second time against a
	// delta the computer has already resolved.
	if l.immobile {
		return
	}
	delta := m.Position().Sub(l.Position())
	l.data.Pos = m.Position()
	l.data.Vel = m.Velocity()
	l.onGround = l.checkOnGround()
	l.updateFallState(delta[1])
	// Blocks that act on an entity standing in them, such as fire and cobweb,
	// are triggered from the collision pass Move used to run. The movement
	// computer does not do it, so it has to happen here or an entity carried
	// into one of those blocks by gravity alone would never set it off.
	l.checkEntityInsiders(l.entityType.BBox(l).Translate(l.data.Pos))
}

// Variant ...
func (l *Living) Variant() int32 {
	return l.variant
}

// WithVariant ...
func (l *Living) WithVariant(v int32) {
	l.variant = v
	l.updateState()
}

// MarkVariant ...
func (l *Living) MarkVariant() int32 {
	return l.markVariant
}

// WithMarkVariant ...
func (l *Living) WithMarkVariant(v int32) {
	l.markVariant = v
	l.updateState()
}

// updateFallState is called to update the entities falling state.
func (l *Living) updateFallState(distanceThisTick float64) {
	if l.OnGround() {
		if l.fallDistance > 0 {
			l.fall(l.fallDistance)
			l.ResetFallDistance()
		}
	} else if distanceThisTick < 0 {
		l.fallDistance += -distanceThisTick
	} else if l.fallDistance > 0 {
		l.ResetFallDistance()
	}
}

// fall is called when a falling entity hits the ground.
func (l *Living) fall(distance float64) {
	pos := cube.PosFromVec3(l.Position())
	b := l.tx.Block(pos)

	if len(b.Model().BBox(pos, l.tx)) == 0 {
		pos = pos.Sub(cube.Pos{0, 1})
		b = l.tx.Block(pos)
	}
	if h, ok := b.(block.EntityLander); ok {
		h.EntityLand(pos, l.tx, l, &distance)
	}
	dmg := distance - 3
	if dmg < 0.5 {
		return
	}
	l.Hurt(math.Ceil(dmg), entity.FallDamageSource{})
}

// calculateCollisionAdjustedMovement calculates movement with collision adjustments and returns the adjusted deltaPos
func (l *Living) calculateCollisionAdjustedMovement(vel mgl64.Vec3) mgl64.Vec3 {
	entityBBox := l.entityType.BBox(l).Translate(l.Position())
	deltaX, deltaY, deltaZ := vel[0], vel[1], vel[2]

	l.checkEntityInsiders(entityBBox)

	// Extend the bounding box by the movement vector to get collision area
	grown := entityBBox.Extend(vel).Grow(0.001)
	low, high := grown.Min(), grown.Max()
	minX, minY, minZ := int(math.Floor(low[0])), int(math.Floor(low[1])), int(math.Floor(low[2]))
	maxX, maxY, maxZ := int(math.Ceil(high[0])), int(math.Ceil(high[1])), int(math.Ceil(high[2]))

	// Collect all collision boxes in the movement area
	blocks := make([]cube.BBox, 0, (maxX-minX+1)*(maxY-minY+1)*(maxZ-minZ+1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				pos := cube.Pos{x, y, z}
				boxes := l.tx.Block(pos).Model().BBox(pos, l.tx)
				for _, box := range boxes {
					blocks = append(blocks, box.Translate(pos.Vec3()))
				}
			}
		}
	}

	// Apply collision detection in proper order: Y first, then X, then Z
	const epsilon = 0.001

	// Y-axis collision (vertical movement)
	if !mgl64.FloatEqualThreshold(deltaY, 0, epsilon) {
		for _, blockBBox := range blocks {
			newDeltaY := entityBBox.YOffset(blockBBox, deltaY)
			if newDeltaY != deltaY {
				deltaY = newDeltaY
			}
		}
		entityBBox = entityBBox.Translate(mgl64.Vec3{0, deltaY, 0})
	}

	// X-axis collision (horizontal movement)
	if !mgl64.FloatEqualThreshold(deltaX, 0, epsilon) {
		for _, blockBBox := range blocks {
			newDeltaX := entityBBox.XOffset(blockBBox, deltaX)
			if newDeltaX != deltaX {
				deltaX = newDeltaX
			}
		}
		entityBBox = entityBBox.Translate(mgl64.Vec3{deltaX, 0, 0})
	}

	// Z-axis collision (horizontal movement)
	if !mgl64.FloatEqualThreshold(deltaZ, 0, epsilon) {
		for _, blockBBox := range blocks {
			newDeltaZ := entityBBox.ZOffset(blockBBox, deltaZ)
			if newDeltaZ != deltaZ {
				deltaZ = newDeltaZ
			}
		}
	}

	// Update collision flags
	l.collidedHorizontally = !mgl64.FloatEqualThreshold(deltaX, vel[0], epsilon) ||
		!mgl64.FloatEqualThreshold(deltaZ, vel[2], epsilon)
	l.collidedVertically = !mgl64.FloatEqualThreshold(deltaY, vel[1], epsilon)

	return mgl64.Vec3{deltaX, deltaY, deltaZ}
}

// checkEntityInsiders checks if the player is colliding with any EntityInsider blocks.
func (l *Living) checkEntityInsiders(entityBBox cube.BBox) {
	box := entityBBox.Grow(-0.0001)
	low, high := cube.PosFromVec3(box.Min()), cube.PosFromVec3(box.Max())

	for y := low[1]; y <= high[1]; y++ {
		for x := low[0]; x <= high[0]; x++ {
			for z := low[2]; z <= high[2]; z++ {
				blockPos := cube.Pos{x, y, z}
				b := l.tx.Block(blockPos)
				if collide, ok := b.(block.EntityInsider); ok {
					collide.EntityInside(blockPos, l.tx, l)
					if _, liquid := b.(world.Liquid); liquid {
						continue
					}
				}

				if lq, ok := l.tx.Liquid(blockPos); ok {
					if collide, ok := lq.(block.EntityInsider); ok {
						collide.EntityInside(blockPos, l.tx, l)
					}
				}
			}
		}
	}
}

// checkOnGround checks if the player is currently considered to be on the ground.
func (l *Living) checkOnGround() bool {
	box := l.entityType.BBox(l).Translate(l.Position())

	// Create a small area below the entity to check for ground
	groundCheck := cube.Box(
		box.Min()[0]-0.001, box.Min()[1]-0.001, box.Min()[2]-0.001,
		box.Max()[0]+0.001, box.Min()[1]+0.001, box.Max()[2]+0.001,
	)

	low, high := cube.PosFromVec3(groundCheck.Min()), cube.PosFromVec3(groundCheck.Max())
	for x := low[0]; x <= high[0]; x++ {
		for z := low[2]; z <= high[2]; z++ {
			for y := low[1]; y <= high[1]; y++ {
				pos := cube.Pos{x, y, z}
				boxList := l.tx.Block(pos).Model().BBox(pos, l.tx)
				for _, bb := range boxList {
					blockBox := bb.Translate(pos.Vec3())
					if blockBox.IntersectsWith(groundCheck) {
						// Check if block surface is at the right height
						if blockBox.Max()[1] >= box.Min()[1]-0.001 &&
							blockBox.Max()[1] <= box.Min()[1]+0.001 {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// Viewers returns the viewers.
func (l *Living) Viewers() []world.Viewer {
	return l.tx.Viewers(l.data.Pos)
}

// insideOfSolid returns true if the player is inside a solid block.
func (l *Living) insideOfSolid() bool {
	pos := cube.PosFromVec3(entity.EyePosition(l))
	b, box := l.tx.Block(pos), l.handle.Type().BBox(l).Translate(l.Position())

	_, solid := b.Model().(model.Solid)
	if !solid {
		// Not solid.
		return false
	}
	d, diffuses := b.(block.LightDiffuser)
	if diffuses && d.LightDiffusionLevel() == 0 {
		// Transparent.
		return false
	}
	for _, blockBox := range b.Model().BBox(pos, l.tx) {
		if blockBox.Translate(pos.Vec3()).IntersectsWith(box) {
			return true
		}
	}
	return false
}

// updateState updates the state of the player to all Viewers of the player.
func (l *Living) updateState() {
	for _, v := range l.Viewers() {
		v.ViewEntityState(l)
	}
}
