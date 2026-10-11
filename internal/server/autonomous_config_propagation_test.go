// B116 — found live 2026-10-11: handlePutConfig's BL369 propagation
// re-marshaled the WHOLE autonomous config struct on every
// autonomous.* write, which clobbered any deliberately-no-omitempty
// bool field (auto_approve_children et al.) whose live-manager default
// diverged from the YAML zero-value. Fixed by scoping the propagated
// JSON to only the top-level field(s) this specific PUT touched.

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dmz006/datawatch/internal/config"
)

type autonomousConfigPropagationSpy struct {
	fakeOrchAutonomous
	lastSetConfig json.RawMessage
	calls         int
}

func (s *autonomousConfigPropagationSpy) SetConfig(v any) error {
	s.calls++
	if raw, ok := v.(json.RawMessage); ok {
		s.lastSetConfig = raw
		return nil
	}
	b, _ := json.Marshal(v)
	s.lastSetConfig = b
	return nil
}

func TestHandlePutConfig_AutonomousPropagation_OnlyTouchedFieldForwarded(t *testing.T) {
	spy := &autonomousConfigPropagationSpy{}
	s := &Server{
		cfg:           &config.Config{},
		cfgPath:       t.TempDir() + "/config.yaml",
		autonomousMgr: spy,
	}
	patch, _ := json.Marshal(map[string]string{"autonomous.worktree_base_repo": "/tmp/repo"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(patch))
	rr := httptest.NewRecorder()
	s.handlePutConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if spy.calls != 1 {
		t.Fatalf("want exactly 1 SetConfig propagation call, got %d", spy.calls)
	}
	var forwarded map[string]json.RawMessage
	if err := json.Unmarshal(spy.lastSetConfig, &forwarded); err != nil {
		t.Fatalf("decode propagated payload: %v", err)
	}
	if _, ok := forwarded["auto_approve_children"]; ok {
		t.Errorf("untouched auto_approve_children must NOT be forwarded (would clobber the live manager's own default), got keys=%v", forwarded)
	}
	if v, ok := forwarded["worktree_base_repo"]; !ok || string(v) != `"/tmp/repo"` {
		t.Errorf("touched worktree_base_repo must be forwarded with its converted value, got present=%v val=%s", ok, v)
	}
	if len(forwarded) != 1 {
		t.Errorf("expected exactly 1 forwarded top-level field, got %d: %v", len(forwarded), forwarded)
	}
}

// Nested dot-path keys (autonomous.default_quality_gates.<field>) must
// forward the whole top-level "default_quality_gates" object, not a
// literal "default_quality_gates.enabled" key.
func TestHandlePutConfig_AutonomousPropagation_NestedKeyForwardsWholeObject(t *testing.T) {
	spy := &autonomousConfigPropagationSpy{}
	s := &Server{
		cfg:           &config.Config{},
		cfgPath:       t.TempDir() + "/config.yaml",
		autonomousMgr: spy,
	}
	patch, _ := json.Marshal(map[string]any{"autonomous.default_quality_gates.enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(patch))
	rr := httptest.NewRecorder()
	s.handlePutConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var forwarded map[string]json.RawMessage
	if err := json.Unmarshal(spy.lastSetConfig, &forwarded); err != nil {
		t.Fatalf("decode propagated payload: %v", err)
	}
	obj, ok := forwarded["default_quality_gates"]
	if !ok {
		t.Fatalf("expected default_quality_gates object forwarded, got keys=%v", forwarded)
	}
	var gates config.QualityGateConfig
	if err := json.Unmarshal(obj, &gates); err != nil {
		t.Fatalf("decode default_quality_gates: %v", err)
	}
	if !gates.Enabled {
		t.Errorf("default_quality_gates.enabled not applied, got %+v", gates)
	}
	if _, ok := forwarded["auto_approve_children"]; ok {
		t.Errorf("untouched auto_approve_children must NOT be forwarded, got keys=%v", forwarded)
	}
}

// No autonomous.* key in the patch at all: SetConfig must not be
// called (nothing for it to clobber).
func TestHandlePutConfig_AutonomousPropagation_NoAutonomousKeyNoCall(t *testing.T) {
	spy := &autonomousConfigPropagationSpy{}
	s := &Server{
		cfg:           &config.Config{},
		cfgPath:       t.TempDir() + "/config.yaml",
		autonomousMgr: spy,
	}
	patch, _ := json.Marshal(map[string]string{"server.host": "0.0.0.0"})
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(patch))
	rr := httptest.NewRecorder()
	s.handlePutConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if spy.calls != 0 {
		t.Fatalf("want 0 SetConfig calls for a non-autonomous patch, got %d", spy.calls)
	}
}
