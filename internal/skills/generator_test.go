package skills

import (
	"encoding/base64"
	"os"
	"path/filepath"
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

	if err := writePromptSkill(dir, p, rendered); err != nil {
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

	// Supporting file must have 0o755 permissions.
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755, got %o", info.Mode().Perm())
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

	if err := writePromptSkill(dir, p, rendered); err != nil {
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

	// Nested file must have 0o755 permissions.
	finfo, _ := os.Stat(filePath)
	if finfo.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755, got %o", finfo.Mode().Perm())
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

	if err := writePromptSkill(dir, p, rendered); err != nil {
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

	if err := writePromptSkill(dir, p, nil); err != nil {
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

	if err := writePromptSkill(dir, p, rendered); err != nil {
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

	err := writePromptSkill(dir, p, rendered)
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}
