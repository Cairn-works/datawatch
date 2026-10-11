// BL406 — found live 2026-10-11: the plan's own promise that "the
// decomposer... reads whichever path is configured" was never
// implemented despite all 6 phases being marked Done.

package autonomous

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesContextSuffix_EmptyProjectDir(t *testing.T) {
	m := &Manager{cfg: Config{}}
	if got := m.rulesContextSuffix(&PRD{}); got != "" {
		t.Fatalf("want empty suffix with no project_dir, got %q", got)
	}
}

func TestRulesContextSuffix_DefaultFilenames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("rule: do not break the build"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{}}
	got := m.rulesContextSuffix(&PRD{ProjectDir: dir})
	if !strings.Contains(got, "do not break the build") {
		t.Fatalf("expected AGENT.md content in suffix, got %q", got)
	}
	if !strings.Contains(got, "AGENT.md") {
		t.Errorf("expected the suffix to name the file it came from, got %q", got)
	}
}

func TestRulesContextSuffix_NoContextFileIsFine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("rule content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// No CONTEXT.md written — must not error or panic, just omit it.
	m := &Manager{cfg: Config{}}
	got := m.rulesContextSuffix(&PRD{ProjectDir: dir})
	if strings.Contains(got, "Project context") {
		t.Errorf("must not claim a context section when no context file exists, got %q", got)
	}
	if !strings.Contains(got, "rule content") {
		t.Errorf("rules file content missing, got %q", got)
	}
}

func TestRulesContextSuffix_CustomFilenames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CUSTOM-RULES.md"), []byte("custom rule text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DATAWATCH-CONTEXT.md"), []byte("project state text"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{RulesFile: "CUSTOM-RULES.md", ContextFile: "DATAWATCH-CONTEXT.md"}}
	got := m.rulesContextSuffix(&PRD{ProjectDir: dir})
	if !strings.Contains(got, "custom rule text") {
		t.Errorf("custom rules file not read, got %q", got)
	}
	if !strings.Contains(got, "project state text") {
		t.Errorf("custom context file not read, got %q", got)
	}
}

func TestRulesContextSuffix_CapsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", rulesContextMaxBytesPerFile+1000)
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{}}
	got := m.rulesContextSuffix(&PRD{ProjectDir: dir})
	if len(got) > rulesContextMaxBytesPerFile+200 {
		t.Errorf("oversized rules file was not capped: suffix length %d", len(got))
	}
}

func TestDecompose_InjectsRulesFileContentIntoPrompt(t *testing.T) {
	baseRepo := scratchGitRepo(t)
	worktreeDir := t.TempDir()

	var capturedPrompt string
	fakeFn := func(req DecomposeRequest) (string, error) {
		capturedPrompt = req.Spec
		return fakePlanResp, nil
	}

	cfg := DefaultConfig()
	cfg.WorktreeBaseRepo = baseRepo
	cfg.WorktreeDir = worktreeDir
	m, err := NewManager(t.TempDir(), cfg, fakeFn)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	prd, err := m.CreatePRD("spec", "", "", "", "")
	if err != nil {
		t.Fatalf("CreatePRD: %v", err)
	}
	if _, err := m.Decompose(prd.ID); err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	prd, _ = m.Store().GetPRD(prd.ID)

	// scratchGitRepo doesn't seed an AGENT.md, so the worktree genuinely
	// has none -- confirm the absence case doesn't break decompose, then
	// seed one and decompose again to confirm presence actually reaches
	// the prompt.
	if strings.Contains(capturedPrompt, "Project rules") {
		t.Fatalf("no AGENT.md exists yet; prompt should not claim one, got %q", capturedPrompt)
	}

	if err := os.WriteFile(filepath.Join(prd.ProjectDir, "AGENT.md"), []byte("MARKER: never break the build"), 0o644); err != nil {
		t.Fatalf("seed AGENT.md: %v", err)
	}
	if _, err := m.Decompose(prd.ID); err != nil {
		t.Fatalf("second Decompose: %v", err)
	}
	if !strings.Contains(capturedPrompt, "MARKER: never break the build") {
		t.Fatalf("AGENT.md content did not reach the decompose prompt, got %q", capturedPrompt)
	}
}
