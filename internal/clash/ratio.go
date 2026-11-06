package clash

import "errors"

var (
	ErrNegativeWeight = errors.New("weight must not be negative")
	ErrZeroWeightSum  = errors.New("weight sum must be greater than zero")
)

type WeightInput struct {
	Key    string
	Weight int64
}

type RatioResult struct {
	Key      string
	Weight   int64
	Ratio    float64
	Included bool
}

type BudgetWeight struct {
	Key         string
	BudgetBytes int64
}

func NormalizeWeights(items []WeightInput) ([]RatioResult, error) {
	var sum int64
	for _, item := range items {
		if item.Weight < 0 {
			return nil, ErrNegativeWeight
		}
		if item.Weight > 0 {
			sum += item.Weight
		}
	}
	if sum <= 0 {
		return nil, ErrZeroWeightSum
	}
	out := make([]RatioResult, len(items))
	for i, item := range items {
		out[i] = RatioResult{
			Key:      item.Key,
			Weight:   item.Weight,
			Included: item.Weight > 0,
		}
		if item.Weight > 0 {
			out[i].Ratio = float64(item.Weight) / float64(sum)
		}
	}
	return out, nil
}

func BudgetWeights(items []BudgetWeight) []WeightInput {
	out := make([]WeightInput, 0, len(items))
	for _, item := range items {
		if item.BudgetBytes <= 0 {
			continue
		}
		out = append(out, WeightInput{Key: item.Key, Weight: item.BudgetBytes})
	}
	return out
}
