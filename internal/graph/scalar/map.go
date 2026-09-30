// Package scalar defines custom GraphQL scalar types.
package scalar

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

// Map is a GraphQL scalar that serialises to/from an arbitrary JSON value
// (object, array, string, number, boolean, or null).
// In Go it is represented as any (interface{}).
type Map = any

// MarshalMap encodes a Map value for a GraphQL response.
func MarshalMap(v Map) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		b, err := json.Marshal(v)
		if err != nil {
			// Fallback: write null on encoding failure.
			_, _ = io.WriteString(w, "null")
			return
		}
		_, _ = w.Write(b)
	})
}

// UnmarshalMap decodes a raw GraphQL variable into a Map value.
func UnmarshalMap(v any) (Map, error) {
	switch val := v.(type) {
	case map[string]any, []any, string, float64, bool, nil:
		return val, nil
	case json.RawMessage:
		var out any
		if err := json.Unmarshal(val, &out); err != nil {
			return nil, fmt.Errorf("scalar.Map: cannot unmarshal JSON: %w", err)
		}
		return out, nil
	default:
		// For any other type, attempt a JSON round-trip.
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("scalar.Map: cannot marshal %T: %w", v, err)
		}
		var out any
		if err = json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("scalar.Map: cannot unmarshal %T: %w", v, err)
		}
		return out, nil
	}
}
