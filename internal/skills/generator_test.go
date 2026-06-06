package skills

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWritePromptSkill_SupportingFiles(t *testing.T) {
	dir := t.TempDir()
	scriptContent := "#!/bin/bash\necho hello"

	p := promptDef{Name: "test-prompt", Description: "A test prompt"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "test content"}},
	}
	rendered.Data.Files = []promptFile{
		{
			Path:    "run.sh",
			Content: base64.StdEncoding.EncodeToString([]byte(scriptContent)),
		},
	}

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	// SKILL.md must exist.
	skillPath := filepath.Join(dir, "dot-ai-test-prompt", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected SKILL.md to exist: %v", err)
	}

	// Supporting file must exist with correct content.
	filePath := filepath.Join(dir, "dot-ai-test-prompt", "run.sh")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected supporting file to exist: %v", err)
	}
	if string(content) != scriptContent {
		t.Errorf("expected %q, got %q", scriptContent, string(content))
	}

	// Supporting file must have 0o755 permissions (Unix only).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("expected permissions 0755, got %o", info.Mode().Perm())
		}
	}
}

func TestWritePromptSkill_NestedPath(t *testing.T) {
	dir := t.TempDir()
	yamlContent := "apiVersion: apps/v1\nkind: Deployment"

	p := promptDef{Name: "nested-prompt", Description: "A prompt with nested files"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "deploy something"}},
	}
	rendered.Data.Files = []promptFile{
		{
			Path:    "templates/deployment.yaml",
			Content: base64.StdEncoding.EncodeToString([]byte(yamlContent)),
		},
	}

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	// Intermediate directory must be created.
	nestedDir := filepath.Join(dir, "dot-ai-nested-prompt", "templates")
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("expected templates/ directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected templates/ to be a directory")
	}

	// Nested file must exist with correct content.
	filePath := filepath.Join(nestedDir, "deployment.yaml")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected nested file to exist: %v", err)
	}
	if string(content) != yamlContent {
		t.Errorf("expected %q, got %q", yamlContent, string(content))
	}

	// Nested file must have 0o755 permissions (Unix only).
	if runtime.GOOS != "windows" {
		finfo, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("stat nested file: %v", err)
		}
		if finfo.Mode().Perm() != 0o755 {
			t.Errorf("expected permissions 0755, got %o", finfo.Mode().Perm())
		}
	}
}

func TestWritePromptSkill_NoFiles(t *testing.T) {
	dir := t.TempDir()

	p := promptDef{Name: "no-files-prompt", Description: "A flat prompt"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "just text"}},
	}
	// No files field set — zero value (empty slice).

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	// SKILL.md must exist.
	skillDir := filepath.Join(dir, "dot-ai-no-files-prompt")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md to exist: %v", err)
	}

	// Directory must contain only SKILL.md.
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected only SKILL.md, got %v", names)
	}
}

func TestWritePromptSkill_NilRendered(t *testing.T) {
	dir := t.TempDir()

	p := promptDef{Name: "nil-prompt", Description: "Prompt with nil render"}

	if err := writePromptSkill(dir, p, nil, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	// SKILL.md must exist with fallback content.
	content, err := os.ReadFile(filepath.Join(dir, "dot-ai-nil-prompt", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected SKILL.md to exist: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty SKILL.md")
	}
}

func TestWritePromptSkill_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	files := []promptFile{
		{
			Path:    "setup.sh",
			Content: base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\nsetup")),
		},
		{
			Path:    "cleanup.sh",
			Content: base64.StdEncoding.EncodeToString([]byte("#!/bin/bash\ncleanup")),
		},
		{
			Path:    "configs/app.yaml",
			Content: base64.StdEncoding.EncodeToString([]byte("key: value")),
		},
	}

	p := promptDef{Name: "multi-prompt", Description: "Multi-file prompt"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "multi"}},
	}
	rendered.Data.Files = files

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	skillDir := filepath.Join(dir, "dot-ai-multi-prompt")

	// All files must exist.
	for _, f := range files {
		decoded, _ := base64.StdEncoding.DecodeString(f.Content)
		filePath := filepath.Join(skillDir, f.Path)
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("expected %s to exist: %v", f.Path, err)
			continue
		}
		if string(content) != string(decoded) {
			t.Errorf("file %s: expected %q, got %q", f.Path, string(decoded), string(content))
		}
	}
}

func TestWritePromptSkill_RewritesFileReferences(t *testing.T) {
	dir := t.TempDir()
	scriptContent := "#!/bin/bash\necho hello"

	p := promptDef{Name: "my-skill", Description: "Skill with file refs"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "Run:\n```bash\nbash ./run.sh\n```\nOr:\n```bash\nbash run.sh\n```\n"}},
	}
	rendered.Data.Files = []promptFile{
		{
			Path:    "run.sh",
			Content: base64.StdEncoding.EncodeToString([]byte(scriptContent)),
		},
	}

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "dot-ai-my-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}

	expected := filepath.Join(dir, "dot-ai-my-skill", "run.sh")
	text := string(content)

	// ./run.sh should be rewritten to full path.
	if !contains(text, "bash "+expected) {
		t.Errorf("expected ./run.sh to be rewritten to %s in SKILL.md:\n%s", expected, text)
	}

	// No bare "run.sh" should remain (both forms rewritten).
	// Count occurrences of the full path — should be 2 (one for each code block).
	count := strings.Count(text, expected)
	if count != 2 {
		t.Errorf("expected 2 occurrences of full path, got %d in:\n%s", count, text)
	}
}

func TestWritePromptSkill_RewritesNestedFileReferences(t *testing.T) {
	dir := t.TempDir()

	p := promptDef{Name: "nested-ref", Description: "Nested file refs"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "Apply:\n```bash\nkubectl apply -f templates/deploy.yaml\n```\n"}},
	}
	rendered.Data.Files = []promptFile{
		{
			Path:    "templates/deploy.yaml",
			Content: base64.StdEncoding.EncodeToString([]byte("apiVersion: v1")),
		},
	}

	if err := writePromptSkill(dir, p, rendered, ""); err != nil {
		t.Fatalf("writePromptSkill: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "dot-ai-nested-ref", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}

	expected := filepath.Join(dir, "dot-ai-nested-ref", "templates", "deploy.yaml")
	if !contains(string(content), expected) {
		t.Errorf("expected templates/deploy.yaml to be rewritten to %s in:\n%s", expected, string(content))
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestWritePromptSkill_InvalidBase64(t *testing.T) {
	dir := t.TempDir()

	p := promptDef{Name: "bad-b64", Description: "Invalid base64"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "test"}},
	}
	rendered.Data.Files = []promptFile{
		{Path: "bad.sh", Content: "!!!not-valid-base64!!!"},
	}

	err := writePromptSkill(dir, p, rendered, "")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestWritePromptSkill_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	p := promptDef{Name: "bad-path", Description: "Invalid path"}
	rendered := &promptRenderResponse{Success: true}
	rendered.Data.Messages = []promptMsg{
		{Role: "user", Content: struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "text", Text: "x"}},
	}

	cases := []struct {
		name string
		path string
	}{
		{"relative traversal", "../escape.sh"},
		{"absolute path", "/etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered.Data.Files = []promptFile{
				{Path: tc.path, Content: base64.StdEncoding.EncodeToString([]byte("echo nope"))},
			}
			err := writePromptSkill(dir, p, rendered, "")
			if err == nil || !strings.Contains(err.Error(), "path traversal") {
				t.Fatalf("expected path traversal error, got %v", err)
			}
		})
	}
}
