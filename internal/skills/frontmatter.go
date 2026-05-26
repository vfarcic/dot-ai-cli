package skills

import (
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// existingSkill records a previously generated skill directory found on disk
// and the source it was tagged with (empty string for legacy pre-PRD-#12
// files that lack a `source:` field).
type existingSkill struct {
	Path   string // absolute path to the skill directory
	Source string // verbatim from frontmatter; "" for untagged legacy files
}

// frontmatterReadCap bounds how many bytes readSkillSource will pull from a
// SKILL.md file. Real frontmatter is a few hundred bytes; the cap defends the
// scan against a hostile or accidentally enormous file from a foreign source.
const frontmatterReadCap = 64 * 1024

// readSkillSource returns the `source:` value from the YAML frontmatter of a
// SKILL.md file. Missing/unparsable frontmatter or a missing source key both
// resolve to "" (untagged) — these are legacy files that predate PRD #12.
// Reads are bounded by frontmatterReadCap.
func readSkillSource(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, frontmatterReadCap))
	if err != nil {
		return ""
	}
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return ""
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		// Either there is no closing marker within the read cap, or the file
		// is malformed. Treat as untagged either way.
		return ""
	}
	var fm struct {
		Source string `yaml:"source"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return ""
	}
	return fm.Source
}
