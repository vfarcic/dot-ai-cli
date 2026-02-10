package main

import (
	_ "embed"

	"github.com/vfarcic/dot-ai-cli/cmd"
)

//go:embed openapi.json
var openapiSpec []byte

func main() {
	cmd.Execute(openapiSpec)
}
