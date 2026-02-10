package formatter

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format converts raw JSON response bytes to the requested output format.
func Format(data []byte, format string) (string, error) {
	switch format {
	case "json":
		return string(data), nil
	case "yaml":
		var obj interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			// Not valid JSON — return raw text.
			return string(data), nil
		}
		out, err := yaml.Marshal(obj)
		if err != nil {
			return string(data), nil
		}
		return strings.TrimRight(string(out), "\n"), nil
	default:
		return "", fmt.Errorf("unsupported output format: %q (valid: json, yaml)", format)
	}
}
