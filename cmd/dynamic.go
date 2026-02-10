package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vfarcic/dot-ai-cli/internal/openapi"
)

// openapiSpec holds the embedded OpenAPI spec bytes.
var openapiSpec []byte

// RegisterDynamicCommands parses the OpenAPI spec and registers cobra
// subcommands on rootCmd.
func RegisterDynamicCommands(spec []byte) {
	openapiSpec = spec

	// Set CLI version from the OpenAPI spec's info.version field.
	rootCmd.Version = specVersion(spec)

	defs, err := openapi.Parse(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse OpenAPI spec: %v\n", err)
		return
	}

	registerCommands(rootCmd, defs)
}

// specVersion extracts the info.version field from an OpenAPI spec.
func specVersion(spec []byte) string {
	var s struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(spec, &s); err != nil {
		return "unknown"
	}
	if s.Info.Version == "" {
		return "unknown"
	}
	return s.Info.Version
}

// paramInfo stores parameter metadata in command annotations for use
// by the HTTP execution layer (M5).
type paramInfo struct {
	Name       string `json:"name"`
	Location   string `json:"location"` // path, query, body
	Positional bool   `json:"positional,omitempty"`
}

// registerCommands creates cobra commands from CommandDefs and adds them
// to root.
func registerCommands(root *cobra.Command, defs []openapi.CommandDef) {
	// Separate top-level commands from subcommands.
	var topDefs, subDefs []openapi.CommandDef
	for _, d := range defs {
		if d.Parent == "" {
			topDefs = append(topDefs, d)
		} else {
			subDefs = append(subDefs, d)
		}
	}

	// Register top-level commands, deduplicating by name.
	topLevel := map[string]*cobra.Command{}
	for _, d := range topDefs {
		name := d.Name
		if _, exists := topLevel[name]; exists {
			name = name + "-" + strings.ToLower(d.Method)
			d.Name = name
		}
		cmd := buildCobraCommand(d)
		topLevel[name] = cmd
		root.AddCommand(cmd)
	}

	// Register subcommands under their parent.
	for _, d := range subDefs {
		parent, exists := topLevel[d.Parent]
		if !exists {
			parent = &cobra.Command{
				Use:   d.Parent,
				Short: strings.ToUpper(d.Parent[:1]) + d.Parent[1:] + " commands",
			}
			topLevel[d.Parent] = parent
			root.AddCommand(parent)
		}
		cmd := buildCobraCommand(d)
		parent.AddCommand(cmd)
	}
}

// buildCobraCommand creates a single cobra.Command from a CommandDef.
func buildCobraCommand(def openapi.CommandDef) *cobra.Command {
	positional, flags := splitParams(def.Params)

	// Build Use string with positional arg placeholders.
	use := def.Name
	for _, p := range positional {
		if p.Required {
			use += fmt.Sprintf(" <%s>", p.Name)
		} else {
			use += fmt.Sprintf(" [%s]", p.Name)
		}
	}

	// Store param metadata for M5.
	infos := buildParamInfos(def.Params, positional)
	paramsJSON, _ := json.Marshal(infos)

	cmd := &cobra.Command{
		Use:   use,
		Short: def.Description,
		Long:  def.Long,
		Annotations: map[string]string{
			"method": def.Method,
			"path":   def.Path,
			"params": string(paramsJSON),
		},
		Args: positionalArgsValidator(positional),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Command execution not yet implemented")
			return nil
		},
	}

	// Register flags.
	for _, p := range flags {
		registerFlag(cmd, p)
	}

	// Add enum validation.
	enums := collectEnumFlags(flags)
	if len(enums) > 0 {
		cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
			return validateEnums(cmd, enums)
		}
	}

	return cmd
}

// splitParams separates parameters into positional args and flags.
// Path params are always positional. A single required string body param
// (with no enum constraint) is promoted to a positional arg.
// Everything else becomes a flag.
func splitParams(params []openapi.ParamDef) (positional, flags []openapi.ParamDef) {
	var pathParams, bodyParams, queryParams []openapi.ParamDef

	for _, p := range params {
		switch p.Location {
		case openapi.ParamLocationPath:
			pathParams = append(pathParams, p)
		case openapi.ParamLocationBody:
			bodyParams = append(bodyParams, p)
		case openapi.ParamLocationQuery:
			queryParams = append(queryParams, p)
		}
	}

	// Path params → always positional.
	positional = append(positional, pathParams...)

	// Promote single required string body param (without enum) to positional.
	promotedName := ""
	var requiredStringBody []openapi.ParamDef
	for _, p := range bodyParams {
		if p.Required && p.Type == "string" && len(p.Enum) == 0 {
			requiredStringBody = append(requiredStringBody, p)
		}
	}
	if len(requiredStringBody) == 1 {
		promotedName = requiredStringBody[0].Name
		positional = append(positional, requiredStringBody[0])
	}

	// Remaining body params → flags.
	for _, p := range bodyParams {
		if p.Name != promotedName {
			flags = append(flags, p)
		}
	}

	// Query params → always flags.
	flags = append(flags, queryParams...)

	return positional, flags
}

// positionalArgsValidator returns a cobra.PositionalArgs function that
// enforces the correct number of positional arguments.
func positionalArgsValidator(positional []openapi.ParamDef) cobra.PositionalArgs {
	if len(positional) == 0 {
		return cobra.NoArgs
	}

	required := 0
	for _, p := range positional {
		if p.Required {
			required++
		}
	}

	total := len(positional)
	if required == total {
		return cobra.ExactArgs(total)
	}
	return cobra.RangeArgs(required, total)
}

// registerFlag adds a flag to cmd based on the parameter definition.
func registerFlag(cmd *cobra.Command, p openapi.ParamDef) {
	desc := p.Description
	if len(p.Enum) > 0 {
		desc += fmt.Sprintf(" (one of: %s)", strings.Join(p.Enum, ", "))
	}

	switch p.Type {
	case "integer":
		cmd.Flags().Int(p.Name, 0, desc)
	case "number":
		cmd.Flags().Float64(p.Name, 0, desc)
	case "boolean":
		cmd.Flags().Bool(p.Name, false, desc)
	default: // string, object, array
		cmd.Flags().String(p.Name, "", desc)
	}

	if p.Required {
		cmd.MarkFlagRequired(p.Name)
	}
}

// enumFlag pairs a flag name with its allowed values.
type enumFlag struct {
	name   string
	values []string
}

// collectEnumFlags returns flags that have enum constraints.
func collectEnumFlags(flags []openapi.ParamDef) []enumFlag {
	var result []enumFlag
	for _, p := range flags {
		if len(p.Enum) > 0 {
			result = append(result, enumFlag{name: p.Name, values: p.Enum})
		}
	}
	return result
}

// validateEnums checks that enum-constrained flag values are valid.
func validateEnums(cmd *cobra.Command, enums []enumFlag) error {
	for _, e := range enums {
		f := cmd.Flags().Lookup(e.name)
		if f == nil || !f.Changed {
			continue
		}
		val := f.Value.String()
		valid := false
		for _, v := range e.values {
			if val == v {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid value %q for flag --%s: must be one of [%s]",
				val, e.name, strings.Join(e.values, ", "))
		}
	}
	return nil
}

// buildParamInfos creates metadata for command annotations. Positional
// params come first (matching arg order), then flag params.
func buildParamInfos(all []openapi.ParamDef, positional []openapi.ParamDef) []paramInfo {
	positionalSet := map[string]bool{}
	for _, p := range positional {
		positionalSet[p.Name] = true
	}

	var infos []paramInfo

	// Positional params first, in order.
	for _, p := range positional {
		infos = append(infos, paramInfo{
			Name:       p.Name,
			Location:   string(p.Location),
			Positional: true,
		})
	}

	// Non-positional params.
	for _, p := range all {
		if !positionalSet[p.Name] {
			infos = append(infos, paramInfo{
				Name:     p.Name,
				Location: string(p.Location),
			})
		}
	}

	return infos
}
