package document

import (
	"fmt"
	"regexp"
)

type FieldRule struct {
	Required bool
	Type     string // "string", "int", "float", "bool", "array"
	Min      *float64
	Max      *float64
	Pattern  *regexp.Regexp
}

type Schema map[string]FieldRule

// Validate validates doc against schema rules.
func (s Schema) Validate(doc Document) error {
	for field, rule := range s {
		val, exists := doc[field]
		if !exists || val == nil {
			if rule.Required {
				return fmt.Errorf("schema error: field %q is required", field)
			}
			continue
		}

		if rule.Type != "" {
			switch rule.Type {
			case "string":
				str, ok := val.(string)
				if !ok {
					return fmt.Errorf("schema error: field %q must be string", field)
				}
				if rule.Pattern != nil && !rule.Pattern.MatchString(str) {
					return fmt.Errorf("schema error: field %q does not match pattern", field)
				}
			case "int":
				switch val.(type) {
				case int, int32, int64, uint, uint32, uint64:
				default:
					return fmt.Errorf("schema error: field %q must be integer", field)
				}
			case "float":
				switch val.(type) {
				case float32, float64, int, int64:
				default:
					return fmt.Errorf("schema error: field %q must be float", field)
				}
			case "bool":
				if _, ok := val.(bool); !ok {
					return fmt.Errorf("schema error: field %q must be bool", field)
				}
			}
		}

		if rule.Min != nil {
			var num float64
			switch n := val.(type) {
			case int:
				num = float64(n)
			case int64:
				num = float64(n)
			case float64:
				num = n
			default:
				num = 0
			}
			if num < *rule.Min {
				return fmt.Errorf("schema error: field %q must be >= %f", field, *rule.Min)
			}
		}

		if rule.Max != nil {
			var num float64
			switch n := val.(type) {
			case int:
				num = float64(n)
			case int64:
				num = float64(n)
			case float64:
				num = n
			default:
				num = 0
			}
			if num > *rule.Max {
				return fmt.Errorf("schema error: field %q must be <= %f", field, *rule.Max)
			}
		}
	}
	return nil
}
