package living

import (
	"slices"
	"time"

	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/world"
)

type Config struct {
	world.EntityType
	*entity.MovementComputer
	Speed, EyeHeight, MaxHealth float64
	Drops                       []Drop
	ImmuneDuration              time.Duration
	Handler

	// Hover is how far above the ground the entity floats, zero for one that
	// walks on it.
	//
	// A hovering entity is held this far above whatever is beneath it and falls
	// when that falls away, so it follows the ground rather than ignoring it.
	// It also counts as standing on the ground while it is at its hover height,
	// which is what keeps it moving at its own speed: an entity in the air is
	// taken to have no purchase and moves at a quarter of it.
	Hover float64
}

func (c Config) Apply(data *world.EntityData) {
	if c.EntityType == nil {
		panic("entity type can't be nil")
	}

	if c.Handler == nil {
		c.Handler = NopHandler{}
	}

	data.Data = &livingData{
		entityType:     c.EntityType,
		mc:             c.MovementComputer,
		speed:          c.Speed,
		eyeHeight:      c.EyeHeight,
		HealthManager:  entity.NewHealthManager(c.MaxHealth, c.MaxHealth),
		drops:          slices.Values(c.Drops),
		scale:          1,
		immuneDuration: c.ImmuneDuration,
		effects:        make(map[effect.Type]effect.Effect),
		handler:        c.Handler,
		hover:          c.Hover,
	}
}
