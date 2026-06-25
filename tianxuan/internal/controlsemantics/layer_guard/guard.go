package layerguard

import controltypes "tianxuan/internal/controlsemantics/types"

type LayerGuard struct {
	ActiveLayers map[controltypes.Layer]bool
}

func New() *LayerGuard {
	return &LayerGuard{ActiveLayers: map[controltypes.Layer]bool{
		controltypes.LayerEquilibrium: true,
	}}
}

func (g *LayerGuard) IsActive(layer controltypes.Layer) bool {
	return g.ActiveLayers[layer]
}

func (g *LayerGuard) Activate(layer controltypes.Layer) {
	g.ActiveLayers[layer] = true
}

func (g *LayerGuard) Deactivate(layer controltypes.Layer) {
	delete(g.ActiveLayers, layer)
}
