package openapi

import (
	"os"
	"testing"
)

func TestParse_RealSpec(t *testing.T) {
	data, err := os.ReadFile("../../openapi.json")
	if err != nil {
		t.Fatalf("failed to read openapi.json: %v", err)
	}

	commands, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) == 0 {
		t.Fatal("expected at least one command, got none")
	}

	// No command should come from a path outside /api/v1/.
	for _, cmd := range commands {
		if cmd.Path == "/" || cmd.Path == "" {
			t.Errorf("unexpected command from non-API path: %q", cmd.Path)
		}
	}
}

func TestParse_ToolPromotion(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/tools/query": {
				"post": {
					"summary": "Execute query",
					"description": "Run a query"
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	cmd := commands[0]
	if cmd.Name != "query" {
		t.Errorf("expected name %q, got %q", "query", cmd.Name)
	}
	if cmd.Parent != "" {
		t.Errorf("expected no parent, got %q", cmd.Parent)
	}
	if cmd.Method != "POST" {
		t.Errorf("expected method POST, got %s", cmd.Method)
	}
}

func TestParse_SubcommandHierarchy(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/resources/kinds": {
				"get": {
					"summary": "List resource kinds"
				}
			},
			"/api/v1/knowledge/ask": {
				"post": {
					"summary": "Ask knowledge base"
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}

	byName := make(map[string]CommandDef)
	for _, c := range commands {
		byName[c.Name] = c
	}

	kinds, ok := byName["kinds"]
	if !ok {
		t.Fatal("missing command 'kinds'")
	}
	if kinds.Parent != "resources" {
		t.Errorf("expected parent %q, got %q", "resources", kinds.Parent)
	}

	ask, ok := byName["ask"]
	if !ok {
		t.Fatal("missing command 'ask'")
	}
	if ask.Parent != "knowledge" {
		t.Errorf("expected parent %q, got %q", "knowledge", ask.Parent)
	}
}

func TestParse_PathParams(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/visualize/{sessionId}": {
				"get": {
					"summary": "Visualize session",
					"parameters": [
						{
							"name": "sessionId",
							"in": "path",
							"required": true,
							"description": "Session identifier",
							"schema": {"type": "string"}
						}
					]
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	cmd := commands[0]
	if cmd.Name != "visualize" {
		t.Errorf("expected name %q, got %q", "visualize", cmd.Name)
	}
	if cmd.Parent != "" {
		t.Errorf("expected no parent, got %q", cmd.Parent)
	}
	if len(cmd.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(cmd.Params))
	}

	p := cmd.Params[0]
	if p.Name != "sessionId" {
		t.Errorf("expected param name %q, got %q", "sessionId", p.Name)
	}
	if p.Location != ParamLocationPath {
		t.Errorf("expected location %q, got %q", ParamLocationPath, p.Location)
	}
	if !p.Required {
		t.Error("expected path param to be required")
	}
	if p.Description != "Session identifier" {
		t.Errorf("expected description %q, got %q", "Session identifier", p.Description)
	}
}

func TestParse_QueryParams(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/resources": {
				"get": {
					"summary": "List resources",
					"parameters": [
						{
							"name": "kind",
							"in": "query",
							"required": true,
							"description": "Resource kind",
							"schema": {"type": "string"}
						},
						{
							"name": "namespace",
							"in": "query",
							"required": false,
							"description": "Namespace filter",
							"schema": {"type": "string"}
						}
					]
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	cmd := commands[0]
	if len(cmd.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(cmd.Params))
	}

	byName := make(map[string]ParamDef)
	for _, p := range cmd.Params {
		byName[p.Name] = p
	}

	kind := byName["kind"]
	if kind.Location != ParamLocationQuery {
		t.Errorf("expected location query, got %q", kind.Location)
	}
	if !kind.Required {
		t.Error("expected kind to be required")
	}

	ns := byName["namespace"]
	if ns.Required {
		t.Error("expected namespace to be optional")
	}
}

func TestParse_BodyParams(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/tools/query": {
				"post": {
					"summary": "Execute query",
					"requestBody": {
						"required": true,
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/queryRequest"
								}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"queryRequest": {
					"type": "object",
					"properties": {
						"intent": {
							"type": "string",
							"description": "Natural language query"
						},
						"limit": {
							"type": "number",
							"description": "Max results"
						}
					},
					"required": ["intent"]
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	cmd := commands[0]
	if len(cmd.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(cmd.Params))
	}

	byName := make(map[string]ParamDef)
	for _, p := range cmd.Params {
		byName[p.Name] = p
	}

	intent := byName["intent"]
	if intent.Location != ParamLocationBody {
		t.Errorf("expected location body, got %q", intent.Location)
	}
	if !intent.Required {
		t.Error("expected intent to be required")
	}
	if intent.Type != "string" {
		t.Errorf("expected type string, got %q", intent.Type)
	}
	if intent.Description != "Natural language query" {
		t.Errorf("expected description %q, got %q", "Natural language query", intent.Description)
	}

	limit := byName["limit"]
	if limit.Required {
		t.Error("expected limit to be optional")
	}
	if limit.Type != "number" {
		t.Errorf("expected type number, got %q", limit.Type)
	}
}

func TestParse_EnumValues(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/items": {
				"get": {
					"summary": "List items",
					"parameters": [
						{
							"name": "status",
							"in": "query",
							"schema": {
								"type": "string",
								"enum": ["active", "inactive", "archived"]
							}
						}
					]
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	p := commands[0].Params[0]
	if len(p.Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(p.Enum))
	}
	expected := []string{"active", "inactive", "archived"}
	for i, v := range expected {
		if p.Enum[i] != v {
			t.Errorf("enum[%d]: expected %q, got %q", i, v, p.Enum[i])
		}
	}
}

func TestParse_SkipsNonAPIPath(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/": {
				"get": {"summary": "MCP SSE stream"},
				"post": {"summary": "MCP JSON-RPC"}
			},
			"/health": {
				"get": {"summary": "Health check"}
			},
			"/api/v1/resources": {
				"get": {"summary": "List resources"}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("expected 1 command (only /api/v1/ paths), got %d", len(commands))
	}
	if commands[0].Name != "resources" {
		t.Errorf("expected name %q, got %q", "resources", commands[0].Name)
	}
}

func TestParse_MultipleMethodsSamePath(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/items": {
				"get": {"summary": "List items"},
				"post": {"summary": "Create item"}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}

	methods := make(map[string]bool)
	for _, c := range commands {
		methods[c.Method] = true
	}
	if !methods["GET"] || !methods["POST"] {
		t.Errorf("expected GET and POST, got %v", methods)
	}
}

func TestParse_EmptyBodySchema(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/tools/version": {
				"post": {
					"summary": "Get version",
					"requestBody": {
						"required": true,
						"content": {
							"application/json": {
								"schema": {
									"$ref": "#/components/schemas/versionRequest"
								}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"versionRequest": {}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	cmd := commands[0]
	if len(cmd.Params) != 0 {
		t.Errorf("expected 0 params for empty schema, got %d", len(cmd.Params))
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := Parse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParse_DeepNestedPath(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/knowledge/source/{sourceIdentifier}": {
				"delete": {
					"summary": "Delete knowledge source",
					"parameters": [
						{
							"name": "sourceIdentifier",
							"in": "path",
							"required": true,
							"schema": {"type": "string"}
						}
					]
				}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	cmd := commands[0]
	if cmd.Parent != "knowledge" {
		t.Errorf("expected parent %q, got %q", "knowledge", cmd.Parent)
	}
	if cmd.Name != "source" {
		t.Errorf("expected name %q, got %q", "source", cmd.Name)
	}
	if cmd.Method != "DELETE" {
		t.Errorf("expected method DELETE, got %s", cmd.Method)
	}
	if len(cmd.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(cmd.Params))
	}
	if cmd.Params[0].Name != "sourceIdentifier" {
		t.Errorf("expected param %q, got %q", "sourceIdentifier", cmd.Params[0].Name)
	}
}

func TestParse_ToolsListNotPromoted(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "test", "version": "1.0.0"},
		"paths": {
			"/api/v1/tools": {
				"get": {"summary": "List tools"}
			}
		}
	}`

	commands, err := Parse([]byte(spec))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	cmd := commands[0]
	if cmd.Name != "tools" {
		t.Errorf("expected name %q, got %q", "tools", cmd.Name)
	}
	if cmd.Parent != "" {
		t.Errorf("expected no parent, got %q", cmd.Parent)
	}
}
