package query

import (
	"encoding/json"
	"fmt"

	"github.com/jmespath/go-jmespath"
)

// Apply evaluates a JMESPath expression against a JSON-shaped copy of value.
func Apply(expression string, value any) (any, error) {
	if expression == "" {
		return value, nil
	}

	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}

	result, err := jmespath.Search(expression, normalized)
	if err != nil {
		return nil, fmt.Errorf("evaluate JMESPath expression: %w", err)
	}
	return result, nil
}

func normalize(value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal query input: %w", err)
	}

	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("unmarshal query input: %w", err)
	}
	return normalized, nil
}
