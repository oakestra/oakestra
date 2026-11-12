package store

import (
	"encoding/json"

	"github.com/oakestra/oakestra/addons/internal/definition"
)

// CloneDefinition returns a deep copy of a definition.
func CloneDefinition(a definition.Addon) definition.Addon {
	b, err := json.Marshal(a)
	if err != nil {
		// Definitions only hold strings, numbers and bools.
		panic(err)
	}
	var out definition.Addon
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	out.Normalize()
	return out
}
