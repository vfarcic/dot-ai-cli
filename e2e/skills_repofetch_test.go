//go:build integration

package e2e_test

import (
	"os/exec"
	"strings"
	"testing"
)

// --- PRD #13 M1: --repo-fetch / --repo-dir / --source-label flag surface ---
//
// M1 is pure CLI surface + validation (no network, clone, or upload). Every
// assertion below is decided in PreRunE — or, for the allowed-but-unimplemented
// path, in the honest M2/M3 RunE stub — so these tests never depend on a server
// call and pass regardless of the pinned mock. runCLI still drives the real
// binary over --server-url, matching the project's integration-test convention.

// Mutual exclusion: at most one of --repo / --repo-fetch / --repo-dir may be
// supplied; combining two names both conflicting flags and exits non-zero.
func TestSkillsGenerate_SourceFlags_MutualExclusion(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantInErr []string
	}{
		{
			"repo+repo-fetch",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-fetch", "https://github.com/orgB/skills"},
			[]string{"--repo", "--repo-fetch", "mutually exclusive"},
		},
		{
			"repo+repo-dir",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo", "--repo-dir", "mutually exclusive"},
		},
		{
			"repo-fetch+repo-dir",
			[]string{"--repo-fetch", "https://github.com/orgB/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo-fetch", "--repo-dir", "mutually exclusive"},
		},
		{
			"repo+repo-fetch+repo-dir",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-fetch", "https://github.com/orgB/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo", "--repo-fetch", "--repo-dir", "mutually exclusive"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for mutually exclusive source flags; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			for _, want := range tc.wantInErr {
				if !strings.Contains(combined, want) {
					t.Errorf("expected mutual-exclusion error to contain %q, got: %s", want, combined)
				}
			}
		})
	}
}

// --repo-dir requires --source-label: a local path is not a stable
// cross-machine identifier, so the error must name the missing flag.
func TestSkillsGenerate_RepoDir_RequiresSourceLabel(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--repo-dir", "/some/skills/dir")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when --repo-dir lacks --source-label; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--source-label") {
		t.Errorf("expected error to name the missing --source-label flag, got: %s", combined)
	}
}

// --source-label requires --repo-dir: a label is meaningless without a local
// directory to apply it to (symmetric with --repo-path/--repo-branch).
func TestSkillsGenerate_SourceLabel_RequiresRepoDir(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when --source-label lacks --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--source-label") || !strings.Contains(combined, "--repo-dir") {
		t.Errorf("expected error naming both --source-label and --repo-dir, got: %s", combined)
	}
}

// --repo-path / --repo-branch are valid qualifiers for --repo-fetch (a
// repo-bearing flag). Validation must NOT reject them; the run then reaches the
// honest M3 stub (clone/upload is not implemented in M1).
func TestSkillsGenerate_RepoFetch_AllowsPathBranch(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-fetch", "https://github.com/orgA/skills", "--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-fetch", "https://github.com/orgA/skills", "--repo-branch", "team-skills"}},
		{"both", []string{"--repo-fetch", "https://github.com/orgA/skills", "--repo-path", "skills", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			combined := stdout + stderr
			// The qualifier rule must accept --repo-fetch, so no qualifier error.
			if strings.Contains(combined, "require --repo") {
				t.Errorf("--repo-path/--repo-branch must be allowed with --repo-fetch; got qualifier error: %s", combined)
			}
			// Validation passed; the run reaches the honest M3 not-implemented stub.
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit from the --repo-fetch M3 stub; stdout: %s stderr: %s", stdout, stderr)
			}
			if !strings.Contains(combined, "not yet implemented") || !strings.Contains(combined, "--repo-fetch") {
				t.Errorf("expected the --repo-fetch 'not yet implemented' stub, got: %s", combined)
			}
		})
	}
}

// --repo-dir validation passes (dir + --source-label), so the run reaches the
// honest M2 not-implemented stub — symmetric with the M3 --repo-fetch coverage
// above. t.TempDir() supplies a real directory so validation never short-circuits.
func TestSkillsGenerate_RepoDir_M2Stub(t *testing.T) {
	repoDir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--agent", "claude-code", "--repo-dir", repoDir, "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit from the --repo-dir M2 stub; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "not yet implemented") || !strings.Contains(combined, "M2") {
		t.Errorf("expected the --repo-dir 'not yet implemented' M2 stub, got: %s", combined)
	}
}

// --repo-path / --repo-branch must be rejected with --repo-dir: a local
// directory takes no subdir/branch qualifier. The qualifier error (not the M2
// stub) must fire.
func TestSkillsGenerate_RepoDir_RejectsPathBranch(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-dir", "/some/skills/dir", "--source-label", "foo", "--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-dir", "/some/skills/dir", "--source-label", "foo", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for --repo-path/--repo-branch with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "require --repo or --repo-fetch") {
				t.Errorf("expected qualifier error (a dir takes no path/branch), got: %s", combined)
			}
		})
	}
}

// Existing behavior preserved: --repo-path / --repo-branch with NONE of the
// repo-bearing flags still errors.
func TestSkillsGenerate_PathBranch_RequireRepoFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-branch", "team-skills"}},
		{"both", []string{"--repo-path", "skills", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for --repo-path/--repo-branch without a repo flag; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "require --repo or --repo-fetch") {
				t.Errorf("expected qualifier error, got: %s", combined)
			}
		})
	}
}

// Sanity: the new flags are documented in the help output.
func TestSkillsGenerate_RepoFetchDir_InHelp(t *testing.T) {
	out, err := exec.Command(binaryPath, "skills", "generate", "--help").Output()
	if err != nil {
		t.Fatalf("expected exit 0 from --help, got error: %v", err)
	}
	s := string(out)
	for _, want := range []string{"--repo-fetch", "--repo-dir", "--source-label"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected help to mention %q", want)
		}
	}
}
