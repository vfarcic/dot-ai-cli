package openapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParamLocation indicates where a CLI parameter originates in the OpenAPI spec.
type ParamLocation string

const (
	ParamLocationPath  ParamLocation = "path"
	ParamLocationQuery ParamLocation = "query"
	ParamLocationBody  ParamLocation = "body"
)

// ParamDef describes a single parameter for a CLI command.
type ParamDef struct {
	Name        string
	Description string
	Type        string // string, number, integer, boolean, object, array
	Required    bool
	Location    ParamLocation
	Enum        []string
}

// CommandDef describes a CLI command derived from an OpenAPI path and method.
type CommandDef struct {
	Name        string // command name, e.g. "query", "kinds"
	Parent      string // parent command name; empty for top-level
	Description string // short description (from OpenAPI summary)
	Long        string // long description (from OpenAPI description)
	Method      string // HTTP method: GET, POST, DELETE, PUT, PATCH
	Path        string // original API path, e.g. /api/v1/tools/query
	Params      []ParamDef
}

const pathPrefix = "/api/v1/"

// excludedPaths lists API paths to omit from CLI command generation.
// These are redundant, internal, or superseded endpoints (see PRD M6).
var excludedPaths = map[string]bool{
	"/api/v1/tools/{toolName}": true, // generic tool execution; duplicates promoted commands
	"/api/v1/tools":            true, // tool discovery; internal/debug only
	"/api/v1/openapi":          true, // returns spec already embedded in binary
	"/api/v1/prompts/{promptName}": true, // replaced by skills generation (M13)
	"/api/v1/prompts":              true, // replaced by skills generation (M13)
}

// Parse parses an OpenAPI 3.0 JSON spec and returns CLI command definitions.
// Only paths under /api/v1/ are processed; all others are ignored.
func Parse(specJSON []byte) ([]CommandDef, error) {
	var spec openAPISpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	p := &parser{spec: &spec}
	return p.buildCommands(), nil
}

// --- internal OpenAPI 3.0 types (minimal, only what we need) ---

type openAPISpec struct {
	Paths      map[string]*pathItem `json:"paths"`
	Components *components          `json:"components"`
}

type pathItem struct {
	Get    *operation `json:"get"`
	Post   *operation `json:"post"`
	Put    *operation `json:"put"`
	Patch  *operation `json:"patch"`
	Delete *operation `json:"delete"`
}

type operation struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	Parameters  []parameter  `json:"parameters"`
	RequestBody *requestBody `json:"requestBody"`
}

type parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // path, query, header, cookie
	Required    bool    `json:"required"`
	Description string  `json:"description"`
	Schema      *schema `json:"schema"`
}

type requestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]mediaType `json:"content"`
}

type mediaType struct {
	Schema *schema `json:"schema"`
}

type schema struct {
	Ref         string             `json:"$ref"`
	Type        schemaType         `json:"type"`
	Description string             `json:"description"`
	Properties  map[string]*schema `json:"properties"`
	Required    []string           `json:"required"`
	Enum        []any              `json:"enum"`
}

// schemaType handles OpenAPI type being either a string ("string") or
// an array (["string", "null"]).  It always stores the first non-null value.
type schemaType string

func (s *schemaType) UnmarshalJSON(data []byte) error {
	// Try string first.
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = schemaType(str)
		return nil
	}
	// Try array of strings.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		for _, v := range arr {
			if v != "null" {
				*s = schemaType(v)
				return nil
			}
		}
		if len(arr) > 0 {
			*s = schemaType(arr[0])
		}
		return nil
	}
	return fmt.Errorf("schema type: expected string or []string, got %s", string(data))
}

type components struct {
	Schemas map[string]*schema `json:"schemas"`
}

// --- parser ---

type parser struct {
	spec *openAPISpec
}

func (p *parser) buildCommands() []CommandDef {
	var commands []CommandDef

	for path, item := range p.spec.Paths {
		if !strings.HasPrefix(path, pathPrefix) {
			continue
		}
		if excludedPaths[path] {
			continue
		}

		for _, m := range []struct {
			name string
			op   *operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"PATCH", item.Patch},
			{"DELETE", item.Delete},
		} {
			if m.op == nil {
				continue
			}
			commands = append(commands, p.buildCommand(path, m.name, m.op))
		}
	}

	return commands
}

func (p *parser) buildCommand(path, method string, op *operation) CommandDef {
	relative := strings.TrimPrefix(path, pathPrefix)
	segments := strings.Split(relative, "/")

	// Separate name segments from path-parameter segments.
	var nameSegments []string
	var pathParams []string
	for _, seg := range segments {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			pathParams = append(pathParams, seg[1:len(seg)-1])
		} else {
			nameSegments = append(nameSegments, seg)
		}
	}

	// Promote named tools to top-level: tools/<name> → <name>.
	// Only when there is an actual name after "tools/" (not just "tools").
	if len(nameSegments) >= 2 && nameSegments[0] == "tools" {
		nameSegments = nameSegments[1:]
	}

	// Derive parent and command name from remaining segments.
	var parent, name string
	switch len(nameSegments) {
	case 0:
		// All segments were path params — shouldn't happen under /api/v1/.
		name = "unknown"
	case 1:
		name = nameSegments[0]
	default:
		parent = nameSegments[0]
		name = strings.Join(nameSegments[1:], "-")
	}

	// Collect parameters.
	var params []ParamDef

	// 1. Path parameters (extracted from the URL pattern).
	for _, pp := range pathParams {
		params = append(params, ParamDef{
			Name:     pp,
			Type:     "string",
			Required: true,
			Location: ParamLocationPath,
		})
	}

	// 2. Operation-level parameters (query params, and enrichments for path params).
	for _, param := range op.Parameters {
		if param.In == "path" {
			// Enrich the path param we already added.
			for i := range params {
				if params[i].Name == param.Name {
					if param.Description != "" {
						params[i].Description = param.Description
					}
					if param.Schema != nil && param.Schema.Type != "" {
						params[i].Type = string(param.Schema.Type)
					}
				}
			}
			continue
		}
		if param.In == "query" {
			pd := ParamDef{
				Name:        param.Name,
				Description: param.Description,
				Required:    param.Required,
				Location:    ParamLocationQuery,
				Type:        "string",
			}
			if param.Schema != nil {
				if param.Schema.Type != "" {
					pd.Type = string(param.Schema.Type)
				}
				pd.Enum = enumToStrings(param.Schema.Enum)
			}
			params = append(params, pd)
		}
	}

	// 3. Request body properties (from application/json schema).
	if op.RequestBody != nil {
		if content, ok := op.RequestBody.Content["application/json"]; ok && content.Schema != nil {
			resolved := p.resolveSchema(content.Schema)
			if resolved != nil && resolved.Properties != nil {
				requiredSet := make(map[string]bool)
				for _, r := range resolved.Required {
					requiredSet[r] = true
				}
				for propName, propSchema := range resolved.Properties {
					prop := p.resolveSchema(propSchema)
					pd := ParamDef{
						Name:     propName,
						Required: requiredSet[propName],
						Location: ParamLocationBody,
						Type:     "string",
					}
					if prop != nil {
						if prop.Type != "" {
							pd.Type = string(prop.Type)
						}
						pd.Description = prop.Description
						pd.Enum = enumToStrings(prop.Enum)
					}
					params = append(params, pd)
				}
			}
		}
	}

	return CommandDef{
		Name:        name,
		Parent:      parent,
		Description: op.Summary,
		Long:        op.Description,
		Method:      method,
		Path:        path,
		Params:      params,
	}
}

func (p *parser) resolveSchema(s *schema) *schema {
	if s == nil || s.Ref == "" {
		return s
	}
	parts := strings.Split(strings.TrimPrefix(s.Ref, "#/"), "/")
	if len(parts) == 3 && parts[0] == "components" && parts[1] == "schemas" {
		if p.spec.Components != nil {
			if resolved, ok := p.spec.Components.Schemas[parts[2]]; ok {
				return p.resolveSchema(resolved)
			}
		}
	}
	return nil
}

func enumToStrings(vals []any) []string {
	if len(vals) == 0 {
		return nil
	}
	result := make([]string, len(vals))
	for i, v := range vals {
		result[i] = fmt.Sprintf("%v", v)
	}
	return result
}
