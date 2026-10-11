# Checkpoint — 2026-10-11

Task: extend scripts/check-plugin-manifests.sh with Manifest v3 checks + fixtures

- Done: PLUGIN_MANIFEST_ROOTS override; docs check split into check_docs (unchanged behavior);
  check_v3 runs for every manifest; header + PASS/FAIL text; 5 valid + 8 invalid fixtures under
  internal/plugins/testdata/manifests; manual verification (valid=0, each invalid=1, default unchanged,
  docs regression ok); bash -n ok; shellcheck not installed; go test ./internal/plugins ok.
- Now: commit.
- Remaining: none.
