package allocation

import (
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
)

// EffectiveWeights resolves a client's weights inside its authorized,
// selectable node set. Missing custom weights are zero; inherit uses explicitly configured node weights.
func EffectiveWeights(cat *catalog.Catalog, clientID int, nodes map[string]NodeWeight, ratio *ClientRatio) ([]RatioResult, error) {
	authorized := cat.AuthorizedNodeKeys(clientID)
	inputs := make([]WeightInput, 0, len(authorized))
	for _, key := range authorized {
		node, ok := cat.NodeByKey(key)
		if !ok || !node.Enabled {
			continue
		}
		weight := nodes[key].DefaultWeight
		if ratio != nil && ratio.Mode == ModeCustom {
			weight = ratio.Weights()[key]
		}
		inputs = append(inputs, WeightInput{Key: key, Weight: weight})
	}
	if len(inputs) == 0 {
		return nil, ErrNoSelectableOutlet
	}
	return NormalizeWeights(inputs)
}

var (
	ErrNoSelectableOutlet = errors.New("no selectable outlet")
	ErrInvalidMode        = errors.New("ratio mode must be inherit or custom")
)

func ValidateMode(mode string) error {
	if mode != ModeInherit && mode != ModeCustom {
		return ErrInvalidMode
	}
	return nil
}

func ValidateWeights(weights map[string]int64) error {
	for _, w := range weights {
		if w < 0 {
			return ErrNegativeWeight
		}
	}
	return nil
}
