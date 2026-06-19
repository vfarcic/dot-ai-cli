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

// --repo-dir end-to-end behavior (read, upload, ?source= render, security
// posture) is covered in skills_repodir_test.go now that M2 is implemented; the
// "not yet implemented" stub it used to hit is gone.

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

// Guard: --install-hook with --repo-dir is refused until M5. BuildHookCommand
// does not yet emit the source flag, so an installed hook would regenerate
// without the source — silently broken. The error must name PRD #13 M5.
func TestSkillsGenerate_RepoDir_InstallHookRejected(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--agent", "claude-code",
		"--install-hook", "--repo-dir", "/some/skills/dir", "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --install-hook with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--install-hook") || !strings.Contains(combined, "M5") {
		t.Errorf("expected an --install-hook M5 not-supported error, got: %s", combined)
	}
}

// Guard: --pull-latest with --repo-dir is refused — --pull-latest forces a
// server-side git pull, which is meaningless for an uploaded local source.
func TestSkillsGenerate_RepoDir_PullLatestRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir,
		"--pull-latest", "--repo-dir", "/some/skills/dir", "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --pull-latest with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--pull-latest") || !strings.Contains(combined, "--repo-dir") {
		t.Errorf("expected a --pull-latest/--repo-dir incompatibility error, got: %s", combined)
	}
}

// Validation: a --source-label with characters outside [A-Za-z0-9._-] is refused
// with a clear usage error (it becomes a server-stored identifier and feeds the
// local:<user>-<label> prefix).
func TestSkillsGenerate_SourceLabel_InvalidCharset(t *testing.T) {
	for _, label := range []string{"bad/label", "has space", "with:colon"} {
		t.Run(label, func(t *testing.T) {
			dir := t.TempDir()
			stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir,
				"--repo-dir", "/some/skills/dir", "--source-label", label)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for invalid --source-label %q; stdout: %s stderr: %s", label, stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "--source-label") || !strings.Contains(combined, "invalid") {
				t.Errorf("expected an invalid --source-label charset error, got: %s", combined)
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
