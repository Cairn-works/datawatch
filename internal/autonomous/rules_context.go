// BL406 — the plan's own Context section promised "the decomposer/
// task-spec generation and the new scanner both read whichever path
// is configured" for autonomous.rules_file/context_file, so an
// operator with a differently-named file (this repo's own
// DATAWATCH-CONTEXT.md) could point the config at it instead of
// renaming it. Found live 2026-10-11, after all 6 of that plan's
// phases had been marked Done, that this was never actually
// implemented: RulesFile/ContextFile had full REST/MCP/CLI/comm
// read-write config plumbing but zero consumers anywhere in the
// codebase. This file closes that gap for the decomposer half.
package autonomous

import (
	"os"
	"path/filepath"
)

// rulesContextMaxBytesPerFile defensively caps how much of a single
// rules/context file gets injected into the planning prompt, so an
// unusually large file can't blow out the LLM's context budget.
// Generous enough that this repo's own ~115KB AGENT.md and ~44KB
// DATAWATCH-CONTEXT.md both fit whole.
const rulesContextMaxBytesPerFile = 256 * 1024

// rulesContextSuffix returns the project's RulesFile (default
// "AGENT.md", always applied once a project_dir exists) and
// ContextFile (default "CONTEXT.md", only when that file actually
// exists — not everyone has one) content, resolved relative to prd's
// own project_dir, formatted for appending to the planning prompt so
// the decomposer sees the project's real rules/state when it's
// actually codebase-aware (a session-based backend). Empty when
// ProjectDir is unset (nothing to read yet, e.g. before worktree
// creation) or neither file exists.
func (m *Manager) rulesContextSuffix(prd *PRD) string {
	if prd.ProjectDir == "" {
		return ""
	}
	m.mu.Lock()
	rulesFile := m.cfg.RulesFile
	contextFile := m.cfg.ContextFile
	m.mu.Unlock()
	if rulesFile == "" {
		rulesFile = "AGENT.md"
	}
	if contextFile == "" {
		contextFile = "CONTEXT.md"
	}
	var out string
	if data, ok := readCapped(filepath.Join(prd.ProjectDir, rulesFile), rulesContextMaxBytesPerFile); ok {
		out += "\n\n--- Project rules (" + rulesFile + ") — follow these when planning ---\n" + data
	}
	if data, ok := readCapped(filepath.Join(prd.ProjectDir, contextFile), rulesContextMaxBytesPerFile); ok {
		out += "\n\n--- Project context (" + contextFile + ") ---\n" + data
	}
	return out
}

// readCapped reads path, truncated to max bytes. ok is false when the
// file doesn't exist or can't be read (never an error to the caller —
// a missing optional context file is expected, not exceptional).
func readCapped(path string, max int) (string, bool) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is joined from operator-configured rules_file/context_file and a PRD's own worktree dir, same trust model as every other project-dir file read in this package
	if err != nil {
		return "", false
	}
	if len(data) > max {
		data = data[:max]
	}
	return string(data), true
}
