package main

import (
	_ "embed"

	"github.com/vfarcic/dot-ai-cli/cmd"
)

var version = "dev"

//go:embed openapi.json
var openapiSpec []byte

//go:embed routing-skill.md
var routingSkill []byte

func main() {
	cmd.Execute(openapiSpec, routingSkill, version)
}
