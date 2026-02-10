package cmd

// openapiSpec holds the embedded OpenAPI spec bytes for dynamic command
// registration. Populated by Execute() and used by RegisterDynamicCommands.
var openapiSpec []byte

// RegisterDynamicCommands parses the OpenAPI spec and registers cobra
// subcommands on rootCmd. This is a stub for M2 — actual implementation
// comes in M3 (parser) and M4 (command generation).
func RegisterDynamicCommands(spec []byte) {
	openapiSpec = spec
}
