package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/openapi"
)

// newTestRoot creates a fresh root command for testing.
func newTestRoot() *cobra.Command {
	return &cobra.Command{Use: "test"}
}

func TestRegisterCommands_TopLevel(t *testing.T) {
	root := newTestRoot()
	defs := []openapi.CommandDef{
		{Name: "query", Description: "Execute query", Method: "POST", Path: "/api/v1/tools/query"},
		{Name: "resources", Description: "List resources", Method: "GET", Path: "/api/v1/resources"},
	}

	registerCommands(root, defs)

	if len(root.Commands()) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(root.Commands()))
	}

	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	if !names["query"] || !names["resources"] {
		t.Errorf("expected query and resources, got %v", names)
	}
}

func TestRegisterCommands_Subcommand(t *testing.T) {
	root := newTestRoot()
	defs := []openapi.CommandDef{
		{Name: "ask", Parent: "knowledge", Description: "Ask knowledge base", Method: "POST", Path: "/api/v1/knowledge/ask"},
	}

	registerCommands(root, defs)

	if len(root.Commands()) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(root.Commands()))
	}

	parent := root.Commands()[0]
	if parent.Name() != "knowledge" {
		t.Errorf("expected parent %q, got %q", "knowledge", parent.Name())
	}
	if parent.Short != "Knowledge commands" {
		t.Errorf("expected short %q, got %q", "Knowledge commands", parent.Short)
	}

	subs := parent.Commands()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subcommand, got %d", len(subs))
	}
	if subs[0].Name() != "ask" {
		t.Errorf("expected subcommand %q, got %q", "ask", subs[0].Name())
	}
}

func TestRegisterCommands_TopLevelAndSubcommand(t *testing.T) {
	root := newTestRoot()
	defs := []openapi.CommandDef{
		{Name: "resources", Description: "List resources", Method: "GET", Path: "/api/v1/resources"},
		{Name: "kinds", Parent: "resources", Description: "List resource kinds", Method: "GET", Path: "/api/v1/resources/kinds"},
	}

	registerCommands(root, defs)

	// "resources" is both a real command and a parent.
	if len(root.Commands()) != 1 {
		t.Fatalf("expected 1 top-level command, got %d", len(root.Commands()))
	}

	res := root.Commands()[0]
	if res.Name() != "resources" {
		t.Errorf("expected %q, got %q", "resources", res.Name())
	}
	if res.Short != "List resources" {
		t.Errorf("expected description from real command, got %q", res.Short)
	}
	if res.Annotations["method"] != "GET" {
		t.Error("expected GET method annotation on resources command")
	}

	subs := res.Commands()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subcommand, got %d", len(subs))
	}
	if subs[0].Name() != "kinds" {
		t.Errorf("expected subcommand %q, got %q", "kinds", subs[0].Name())
	}
}

func TestRegisterCommands_DuplicateNameDeduplicated(t *testing.T) {
	root := newTestRoot()
	defs := []openapi.CommandDef{
		{Name: "items", Description: "List items", Method: "GET", Path: "/api/v1/items"},
		{Name: "items", Description: "Create item", Method: "POST", Path: "/api/v1/items"},
	}

	registerCommands(root, defs)

	if len(root.Commands()) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(root.Commands()))
	}

	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	if !names["items"] || !names["items-post"] {
		t.Errorf("expected items and items-post, got %v", names)
	}
}

func TestBuildCobraCommand_PathParamPositional(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "visualize",
		Method: "GET",
		Path:   "/api/v1/visualize/{sessionId}",
		Params: []openapi.ParamDef{
			{Name: "sessionId", Type: "string", Required: true, Location: openapi.ParamLocationPath, Description: "Session identifier"},
		},
	}

	cmd := buildCobraCommand(def)

	if cmd.Use != "visualize <sessionId>" {
		t.Errorf("expected Use %q, got %q", "visualize <sessionId>", cmd.Use)
	}

	// Should accept exactly 1 arg.
	if err := cmd.Args(cmd, []string{"sess-123"}); err != nil {
		t.Errorf("expected 1 arg to be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("expected 0 args to be invalid")
	}
}

func TestBuildCobraCommand_SingleRequiredStringBodyPromoted(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "query",
		Method: "POST",
		Path:   "/api/v1/tools/query",
		Params: []openapi.ParamDef{
			{Name: "intent", Type: "string", Required: true, Location: openapi.ParamLocationBody, Description: "Natural language query"},
			{Name: "limit", Type: "number", Required: false, Location: openapi.ParamLocationBody, Description: "Max results"},
		},
	}

	cmd := buildCobraCommand(def)

	if cmd.Use != "query <intent>" {
		t.Errorf("expected Use %q, got %q", "query <intent>", cmd.Use)
	}

	// "intent" should not be a flag (it's positional).
	if cmd.Flags().Lookup("intent") != nil {
		t.Error("promoted param 'intent' should not be a flag")
	}

	// "limit" should be a flag.
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected 'limit' flag")
	}
}

func TestBuildCobraCommand_MultipleRequiredBodyNotPromoted(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "operate",
		Method: "POST",
		Path:   "/api/v1/tools/operate",
		Params: []openapi.ParamDef{
			{Name: "intent", Type: "string", Required: true, Location: openapi.ParamLocationBody},
			{Name: "target", Type: "string", Required: true, Location: openapi.ParamLocationBody},
		},
	}

	cmd := buildCobraCommand(def)

	if cmd.Use != "operate" {
		t.Errorf("expected Use %q, got %q", "operate", cmd.Use)
	}
	if cmd.Flags().Lookup("intent") == nil {
		t.Error("expected 'intent' flag")
	}
	if cmd.Flags().Lookup("target") == nil {
		t.Error("expected 'target' flag")
	}
}

func TestBuildCobraCommand_FlagTypes(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "test",
		Method: "POST",
		Path:   "/api/v1/test",
		Params: []openapi.ParamDef{
			{Name: "name", Type: "string", Location: openapi.ParamLocationQuery},
			{Name: "count", Type: "integer", Location: openapi.ParamLocationQuery},
			{Name: "ratio", Type: "number", Location: openapi.ParamLocationQuery},
			{Name: "verbose", Type: "boolean", Location: openapi.ParamLocationQuery},
			{Name: "data", Type: "object", Location: openapi.ParamLocationBody},
			{Name: "tags", Type: "array", Location: openapi.ParamLocationBody},
		},
	}

	cmd := buildCobraCommand(def)

	tests := []struct {
		name     string
		flagType string
	}{
		{"name", "string"},
		{"count", "int"},
		{"ratio", "float64"},
		{"verbose", "bool"},
		{"data", "string"},
		{"tags", "string"},
	}

	for _, tt := range tests {
		f := cmd.Flags().Lookup(tt.name)
		if f == nil {
			t.Errorf("expected flag %q", tt.name)
			continue
		}
		if f.Value.Type() != tt.flagType {
			t.Errorf("flag %q: expected type %q, got %q", tt.name, tt.flagType, f.Value.Type())
		}
	}
}

func TestBuildCobraCommand_EnumValidation(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "items",
		Method: "GET",
		Path:   "/api/v1/items",
		Params: []openapi.ParamDef{
			{Name: "status", Type: "string", Location: openapi.ParamLocationQuery, Enum: []string{"active", "inactive"}},
		},
	}

	cmd := buildCobraCommand(def)

	// Valid value should pass.
	cmd.Flags().Set("status", "active")
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Errorf("expected valid enum to pass: %v", err)
	}

	// Invalid value should fail.
	cmd.Flags().Set("status", "bad")
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected invalid enum to fail")
	} else if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("expected 'must be one of' in error, got %q", err.Error())
	}
}

func TestBuildCobraCommand_EnumInDescription(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "items",
		Method: "GET",
		Path:   "/api/v1/items",
		Params: []openapi.ParamDef{
			{Name: "status", Type: "string", Location: openapi.ParamLocationQuery, Description: "Filter by status", Enum: []string{"active", "inactive"}},
		},
	}

	cmd := buildCobraCommand(def)
	f := cmd.Flags().Lookup("status")
	if !strings.Contains(f.Usage, "one of: active, inactive") {
		t.Errorf("expected enum values in description, got %q", f.Usage)
	}
}

func TestBuildCobraCommand_NoEnumNoPreRunE(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "simple",
		Method: "GET",
		Path:   "/api/v1/simple",
		Params: []openapi.ParamDef{
			{Name: "name", Type: "string", Location: openapi.ParamLocationQuery},
		},
	}

	cmd := buildCobraCommand(def)
	if cmd.PreRunE != nil {
		t.Error("expected no PreRunE when no enum flags exist")
	}
}

func TestBuildCobraCommand_Annotations(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "query",
		Method: "POST",
		Path:   "/api/v1/tools/query",
		Params: []openapi.ParamDef{
			{Name: "intent", Type: "string", Required: true, Location: openapi.ParamLocationBody},
		},
	}

	cmd := buildCobraCommand(def)

	if cmd.Annotations["method"] != "POST" {
		t.Errorf("expected method POST, got %q", cmd.Annotations["method"])
	}
	if cmd.Annotations["path"] != "/api/v1/tools/query" {
		t.Errorf("expected path, got %q", cmd.Annotations["path"])
	}
	if cmd.Annotations["params"] == "" {
		t.Error("expected params annotation")
	}
}

func TestBuildCobraCommand_HelpText(t *testing.T) {
	def := openapi.CommandDef{
		Name:        "query",
		Description: "Execute a query",
		Long:        "Run a natural language query against the cluster",
		Method:      "POST",
		Path:        "/api/v1/tools/query",
	}

	cmd := buildCobraCommand(def)

	if cmd.Short != "Execute a query" {
		t.Errorf("expected Short %q, got %q", "Execute a query", cmd.Short)
	}
	if cmd.Long != "Run a natural language query against the cluster" {
		t.Errorf("expected Long %q, got %q", "Run a natural language query against the cluster", cmd.Long)
	}
}

func TestBuildCobraCommand_NoArgsWhenNoParams(t *testing.T) {
	def := openapi.CommandDef{
		Name:   "version",
		Method: "POST",
		Path:   "/api/v1/tools/version",
	}

	cmd := buildCobraCommand(def)

	if cmd.Use != "version" {
		t.Errorf("expected Use %q, got %q", "version", cmd.Use)
	}
	if err := cmd.Args(cmd, []string{}); err != nil {
		t.Errorf("expected 0 args to be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{"extra"}); err == nil {
		t.Error("expected extra args to be invalid")
	}
}

func TestSplitParams_PathAlwaysPositional(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "id", Location: openapi.ParamLocationPath, Required: true},
		{Name: "filter", Location: openapi.ParamLocationQuery},
	}

	pos, flags := splitParams(params)

	if len(pos) != 1 || pos[0].Name != "id" {
		t.Errorf("expected path param as positional, got %+v", pos)
	}
	if len(flags) != 1 || flags[0].Name != "filter" {
		t.Errorf("expected query param as flag, got %+v", flags)
	}
}

func TestSplitParams_SingleRequiredStringPromoted(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "intent", Type: "string", Required: true, Location: openapi.ParamLocationBody},
		{Name: "limit", Type: "number", Required: false, Location: openapi.ParamLocationBody},
	}

	pos, flags := splitParams(params)

	if len(pos) != 1 || pos[0].Name != "intent" {
		t.Errorf("expected intent as positional, got %+v", pos)
	}
	if len(flags) != 1 || flags[0].Name != "limit" {
		t.Errorf("expected limit as flag, got %+v", flags)
	}
}

func TestSplitParams_MultipleRequiredNotPromoted(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "a", Type: "string", Required: true, Location: openapi.ParamLocationBody},
		{Name: "b", Type: "string", Required: true, Location: openapi.ParamLocationBody},
	}

	pos, flags := splitParams(params)

	if len(pos) != 0 {
		t.Errorf("expected no positional, got %+v", pos)
	}
	if len(flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(flags))
	}
}

func TestSplitParams_EnumNotPromoted(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "mode", Type: "string", Required: true, Location: openapi.ParamLocationBody, Enum: []string{"a", "b"}},
	}

	pos, flags := splitParams(params)

	if len(pos) != 0 {
		t.Errorf("expected no positional for enum param, got %+v", pos)
	}
	if len(flags) != 1 {
		t.Errorf("expected 1 flag, got %d", len(flags))
	}
}

func TestSplitParams_NonStringNotPromoted(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "count", Type: "integer", Required: true, Location: openapi.ParamLocationBody},
	}

	pos, flags := splitParams(params)

	if len(pos) != 0 {
		t.Errorf("expected no positional for non-string param, got %+v", pos)
	}
	if len(flags) != 1 {
		t.Errorf("expected 1 flag, got %d", len(flags))
	}
}

func TestSplitParams_PathAndPromotedBody(t *testing.T) {
	params := []openapi.ParamDef{
		{Name: "sessionId", Type: "string", Required: true, Location: openapi.ParamLocationPath},
		{Name: "intent", Type: "string", Required: true, Location: openapi.ParamLocationBody},
		{Name: "mode", Type: "string", Required: false, Location: openapi.ParamLocationBody},
	}

	pos, flags := splitParams(params)

	if len(pos) != 2 {
		t.Fatalf("expected 2 positional, got %d", len(pos))
	}
	if pos[0].Name != "sessionId" {
		t.Errorf("expected first positional %q, got %q", "sessionId", pos[0].Name)
	}
	if pos[1].Name != "intent" {
		t.Errorf("expected second positional %q, got %q", "intent", pos[1].Name)
	}
	if len(flags) != 1 || flags[0].Name != "mode" {
		t.Errorf("expected mode as flag, got %+v", flags)
	}
}

func TestBuildParamInfos_Order(t *testing.T) {
	all := []openapi.ParamDef{
		{Name: "intent", Location: openapi.ParamLocationBody},
		{Name: "sessionId", Location: openapi.ParamLocationPath},
		{Name: "limit", Location: openapi.ParamLocationQuery},
	}
	positional := []openapi.ParamDef{
		{Name: "sessionId", Location: openapi.ParamLocationPath},
		{Name: "intent", Location: openapi.ParamLocationBody},
	}

	infos := buildParamInfos(all, positional)

	if len(infos) != 3 {
		t.Fatalf("expected 3 infos, got %d", len(infos))
	}

	// Positional params first, in order.
	if infos[0].Name != "sessionId" || !infos[0].Positional {
		t.Errorf("expected first info to be sessionId (positional), got %+v", infos[0])
	}
	if infos[1].Name != "intent" || !infos[1].Positional {
		t.Errorf("expected second info to be intent (positional), got %+v", infos[1])
	}
	// Then non-positional.
	if infos[2].Name != "limit" || infos[2].Positional {
		t.Errorf("expected third info to be limit (flag), got %+v", infos[2])
	}
}

func TestSpecVersion(t *testing.T) {
	spec := `{"info": {"version": "1.2.3"}}`
	if v := specVersion([]byte(spec)); v != "1.2.3" {
		t.Errorf("expected %q, got %q", "1.2.3", v)
	}
}

func TestSpecVersion_InvalidJSON(t *testing.T) {
	if v := specVersion([]byte("bad")); v != "unknown" {
		t.Errorf("expected %q, got %q", "unknown", v)
	}
}

func TestSpecVersion_MissingVersion(t *testing.T) {
	if v := specVersion([]byte(`{"info": {}}`)); v != "unknown" {
		t.Errorf("expected %q, got %q", "unknown", v)
	}
}

func TestRegisterDynamicCommands_InvalidSpec(t *testing.T) {
	// Save and restore rootCmd.
	saved := rootCmd
	rootCmd = newTestRoot()
	defer func() { rootCmd = saved }()

	// Should not panic.
	RegisterDynamicCommands([]byte("not json"))

	if len(rootCmd.Commands()) != 0 {
		t.Errorf("expected no commands for invalid spec, got %d", len(rootCmd.Commands()))
	}
}

func TestRegisterCommands_RealSpec(t *testing.T) {
	data, err := os.ReadFile("../openapi.json")
	if err != nil {
		t.Fatalf("failed to read openapi.json: %v", err)
	}

	defs, err := openapi.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	root := newTestRoot()
	registerCommands(root, defs)

	if len(root.Commands()) == 0 {
		t.Fatal("expected commands from real spec")
	}

	// Collect all command names (including subcommands).
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
		for _, sub := range c.Commands() {
			names[c.Name()+"/"+sub.Name()] = true
		}
	}

	// Known commands that should exist from the real spec.
	expected := []string{"query", "version", "resources"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected command %q, not found in %v", name, names)
		}
	}
}
