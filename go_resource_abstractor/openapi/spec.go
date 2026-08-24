// Package openapi holds the service's API contract: openapi.yaml is the
// source of truth, and openapi.gen.go is generated from it by oapi-codegen -
// models, typed path/query params, and the gin ServerInterface the api
// package implements. Adding or changing an endpoint means editing
// openapi.yaml and regenerating, not hand-wiring routes.
//
// Regenerate with `go generate ./openapi` after editing openapi.yaml. The
// generator version is pinned in the directive below, not go.mod, so
// building or deploying never has to resolve it.
package openapi

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config cfg.yaml openapi.yaml

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"
)

// SpecYAML is openapi.yaml verbatim, served at /docs/openapi.yaml.
//
//go:embed openapi.yaml
var SpecYAML []byte

// SpecJSON returns the spec as JSON, served at /docs/openapi.json - the same
// path the Python service published it under, so existing tooling pointed at
// that URL keeps working.
//
// The conversion runs at most once per process, on first request.
var SpecJSON = sync.OnceValues(func() ([]byte, error) {
	var doc any
	if err := yaml.Unmarshal(SpecYAML, &doc); err != nil {
		return nil, fmt.Errorf("parse embedded openapi.yaml: %w", err)
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode openapi spec as json: %w", err)
	}
	return body, nil
})
