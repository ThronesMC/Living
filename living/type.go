package living

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type NopLivingType struct{}

func (n NopLivingType) Open(tx *world.Tx, handle *world.EntityHandle, data *world.EntityData) world.Entity {
	l := &Living{
		livingData: data.Data.(*livingData),
		tx:         tx,
		handle:     handle,
		data:       data,
	}

	return l
}

func (NopLivingType) EncodeEntity() string {
	panic("implement me")
}

func (NopLivingType) BBox(e world.Entity) cube.BBox {
	return cube.BBox{}
}

// DecodeNBT is deliberately a no-op. EntityData.Data carries live gameplay
// state that callers type-assert directly (*SpellData, bool flags, int
// counters), several without the comma-ok form, so writing the saved NBT map
// into it would panic as soon as a loaded entity was ticked. Nothing stored
// here needs to survive a restart.
func (NopLivingType) DecodeNBT(map[string]any, *world.EntityData) {}

// EncodeNBT persists nothing of its own: dragonfly already stores the generic
// entity fields (Pos, Vel, Rot, Name, FireDuration, Age) itself.
//
// It previously returned map[string]any{"data": data}, handing the whole
// EntityData struct to the NBT encoder. EntityData.Pos is an mgl64.Vec3, which
// has no NBT representation, so every attempt to store an entity failed with
// "store entities: encode NBT". That is not cosmetic: a chunk being unloaded
// saves its entities, and when the save fails the entities are lost - which is
// what made spawned towers disappear a minute or two after the map loaded.
func (NopLivingType) EncodeNBT(*world.EntityData) map[string]any { return nil }
