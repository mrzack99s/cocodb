package executor

import (
	"fmt"
)

type AggType uint8

const (
	AggCount AggType = iota + 1
	AggSum
	AggAvg
	AggMin
	AggMax
)

func (a AggType) String() string {
	switch a {
	case AggCount:
		return "count"
	case AggSum:
		return "sum"
	case AggAvg:
		return "avg"
	case AggMin:
		return "min"
	case AggMax:
		return "max"
	default:
		return fmt.Sprintf("agg_%d", a)
	}
}

type AggDef struct {
	Type  AggType
	Field string
	As    string
}

type AggResult struct {
	GroupKey map[string]any
	Values   map[string]any
}

// ComputeAggregation computes aggregates optionally grouped by groupFields.
func ComputeAggregation(op Operator, groupFields []string, aggs []AggDef) ([]AggResult, error) {
	defer op.Close()

	type groupAccum struct {
		count int64
		sums  map[string]float64
		mins  map[string]float64
		maxs  map[string]float64
	}

	groups := make(map[string]*groupAccum)
	groupKeyMaps := make(map[string]map[string]any)

	for op.Next() {
		view := op.View()

		var gKeyStr string
		gKeyMap := make(map[string]any, len(groupFields))
		for _, gf := range groupFields {
			val, _ := view.Get(gf)
			gKeyStr += fmt.Sprintf("%v|", val)
			gKeyMap[gf] = val
		}

		accum, ok := groups[gKeyStr]
		if !ok {
			accum = &groupAccum{
				sums: make(map[string]float64),
				mins: make(map[string]float64),
				maxs: make(map[string]float64),
			}
			groups[gKeyStr] = accum
			groupKeyMaps[gKeyStr] = gKeyMap
		}

		accum.count++

		for _, agg := range aggs {
			if agg.Type == AggCount {
				continue
			}
			fVal, ok := view.Float64(agg.Field)
			if !ok {
				continue
			}

			accum.sums[agg.Field] += fVal

			if curMin, ok := accum.mins[agg.Field]; !ok || fVal < curMin {
				accum.mins[agg.Field] = fVal
			}
			if curMax, ok := accum.maxs[agg.Field]; !ok || fVal > curMax {
				accum.maxs[agg.Field] = fVal
			}
		}
	}

	if err := op.Err(); err != nil {
		return nil, err
	}

	results := make([]AggResult, 0, len(groups))
	for gKey, accum := range groups {
		vals := make(map[string]any, len(aggs))
		for _, agg := range aggs {
			alias := agg.As
			if alias == "" {
				alias = fmt.Sprintf("%s_%s", agg.Field, agg.Type.String())
			}
			switch agg.Type {
			case AggCount:
				vals[alias] = accum.count
			case AggSum:
				vals[alias] = accum.sums[agg.Field]
			case AggAvg:
				if accum.count > 0 {
					vals[alias] = accum.sums[agg.Field] / float64(accum.count)
				} else {
					vals[alias] = 0.0
				}
			case AggMin:
				vals[alias] = accum.mins[agg.Field]
			case AggMax:
				vals[alias] = accum.maxs[agg.Field]
			}
		}

		results = append(results, AggResult{
			GroupKey: groupKeyMaps[gKey],
			Values:   vals,
		})
	}

	return results, nil
}
