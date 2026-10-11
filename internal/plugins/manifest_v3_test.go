// BL403 Phase 1 — Plugin Manifest v3 (kind / connection / lifecycle) tests.

package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseManifest(t *testing.T, src string) Manifest {
	t.Helper()
	var m Manifest
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m.Normalize()
	return m
}

const validService = `name: imap-mcp
kind: service
connection:
  base_url: http://127.0.0.1:8765
  health_path: /healthz
  auth:
    type: bearer
    token: "${secret:imap-mcp-token}"
lifecycle:
  supported_modes: [attach_only, managed]
  entry: ./imap-mcp
  args: [serve, --port, "8765"]
  restart_policy: on-failure
  stop_timeout_ms: 5000
`

func TestManifest_V21DefaultsToAutomation(t *testing.T) {
	m := parseManifest(t, "name: legacy\nentry: ./p.sh\nhooks: [on_alert]\nmode: long-lived\n")
	if m.Kind != KindAutomation {
		t.Fatalf("Kind = %q, want %q", m.Kind, KindAutomation)
	}
	if m.Mode != "long-lived" {
		t.Fatalf("deprecated Mode should still parse, got %q", m.Mode)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("v2.1 manifest should validate: %v", err)
	}
}

func TestManifest_AutomationKeepsMinimalRequirements(t *testing.T) {
	// Pre-v3 Discover only required name — no entry/hooks requirement.
	if err := parseManifest(t, "name: bare\n").Validate(); err != nil {
		t.Fatalf("name-only automation manifest should validate: %v", err)
	}
}

func TestManifest_NormalizeKind(t *testing.T) {
	m := parseManifest(t, "name: x\nkind: '  SERVICE '\nconnection:\n  base_url: https://h\n")
	if m.Kind != KindService {
		t.Fatalf("Kind = %q, want %q", m.Kind, KindService)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestManifest_UnknownKeysTolerated(t *testing.T) {
	m := parseManifest(t, validService+"future_field: 42\nconnection_extra: {a: b}\n")
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown keys should be tolerated: %v", err)
	}
}

func TestManifest_ValidService(t *testing.T) {
	m := parseManifest(t, validService)
	if err := m.Validate(); err != nil {
		t.Fatalf("valid service manifest: %v", err)
	}
	if m.Connection.BaseURL != "http://127.0.0.1:8765" || m.Connection.HealthPath != "/healthz" {
		t.Fatalf("connection not parsed: %+v", m.Connection)
	}
	if m.Connection.Auth == nil || m.Connection.Auth.Token != "${secret:imap-mcp-token}" {
		t.Fatalf("auth not parsed: %+v", m.Connection.Auth)
	}
	l := m.Lifecycle
	if len(l.SupportedModes) != 2 || l.Entry != "./imap-mcp" || len(l.Args) != 3 ||
		l.RestartPolicy != RestartOnFailure || l.StopTimeoutMs != 5000 {
		t.Fatalf("lifecycle not parsed: %+v", l)
	}
}

func TestManifest_ServiceAttachOnlyNoLifecycleNoAuth(t *testing.T) {
	m := parseManifest(t, "name: s\nkind: service\nconnection:\n  base_url: https://svc.example:9000/api\n")
	if err := m.Validate(); err != nil {
		t.Fatalf("connection-only service should validate: %v", err)
	}
}

func TestManifest_ValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"missing name", "kind: automation\n", []string{"name is required"}},
		{"bad kind", "name: x\nkind: daemon\n", []string{`kind "daemon" is invalid`}},
		{"service no connection", "name: x\nkind: service\n", []string{"connection is required"}},
		{"service no base_url", "name: x\nkind: service\nconnection:\n  health_path: /h\n", []string{"connection.base_url is required"}},
		{"relative base_url", "name: x\nkind: service\nconnection:\n  base_url: /just/a/path\n", []string{"absolute http or https URL"}},
		{"ftp base_url", "name: x\nkind: service\nconnection:\n  base_url: ftp://h/\n", []string{"absolute http or https URL"}},
		{"unparseable base_url", "name: x\nkind: service\nconnection:\n  base_url: \"http://h:bad port\"\n", []string{"connection.base_url"}},
		{"bad auth type", "name: x\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    type: basic\n    token: \"${secret:t}\"\n",
			[]string{`auth.type "basic" is unsupported`}},
		{"missing token", "name: x\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    type: bearer\n",
			[]string{"connection.auth.token is required"}},
		{"literal token", "name: x\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    token: sk-live-abc123\n",
			[]string{"single ${secret:name} reference"}},
		{"token with prefix", "name: x\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    token: \"Bearer ${secret:t}\"\n",
			[]string{"single ${secret:name} reference"}},
		{"lifecycle on automation", "name: x\nlifecycle:\n  supported_modes: [attach_only]\n",
			[]string{"lifecycle is only allowed when kind is service"}},
		{"empty modes", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  restart_policy: always\n",
			[]string{"supported_modes must not be empty"}},
		{"bad mode", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  supported_modes: [forced]\n",
			[]string{`"forced" is invalid`}},
		{"dup mode", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  supported_modes: [attach_only, attach_only]\n",
			[]string{"listed more than once"}},
		{"managed no entry", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  supported_modes: [managed]\n",
			[]string{"lifecycle.entry is required"}},
		{"bad restart", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  supported_modes: [attach_only]\n  restart_policy: sometimes\n",
			[]string{`restart_policy "sometimes" is invalid`}},
		{"negative stop timeout", "name: x\nkind: service\nconnection:\n  base_url: http://h\nlifecycle:\n  supported_modes: [attach_only]\n  stop_timeout_ms: -1\n",
			[]string{"stop_timeout_ms must be >= 0"}},
		{"joins every problem", "kind: service\nconnection:\n  auth:\n    token: plain\nlifecycle:\n  supported_modes: [managed, bogus]\n  restart_policy: x\n  stop_timeout_ms: -5\n",
			[]string{"name is required", "base_url is required", "single ${secret:name} reference",
				`"bogus" is invalid`, "lifecycle.entry is required", "restart_policy", "stop_timeout_ms"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseManifest(t, tc.src).Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q missing %q", err.Error(), w)
				}
			}
		})
	}
}

func TestManifest_LiteralTokenNotEchoed(t *testing.T) {
	err := parseManifest(t, "name: x\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    token: sk-live-SECRET\n").Validate()
	if err == nil || strings.Contains(err.Error(), "sk-live-SECRET") {
		t.Fatalf("literal token must not appear in the error: %v", err)
	}
}

func TestRegistry_Discover_ServiceAndInvalid(t *testing.T) {
	root := t.TempDir()
	makePlugin(t, root, "legacy",
		"#!/bin/sh\nread line\necho '{\"action\":\"pass\"}'\n",
		[]string{"post_session_output"})
	for name, src := range map[string]string{
		"svc": validService,
		"bad": "name: bad\nkind: service\n",
	} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeManifest(t, dir, src)
	}
	r, err := NewRegistry(Config{Enabled: true, Dir: root})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, ok := r.Get("bad"); ok {
		t.Fatal("invalid service manifest should be skipped")
	}
	legacy, ok := r.Get("legacy")
	if !ok || legacy.Kind != KindAutomation {
		t.Fatalf("legacy plugin should load as automation: %+v", legacy)
	}
	svc, ok := r.Get("imap-mcp")
	if !ok || svc.Kind != KindService {
		t.Fatalf("service plugin should load: %+v", svc)
	}
}

func TestRegistry_ServicePluginNeverInvoked(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "svc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Even if a service manifest lists hooks, they must be ignored.
	writeManifest(t, dir, validService+"hooks: [post_session_output]\nentry: ./nope.sh\n")
	r, _ := NewRegistry(Config{Enabled: true, Dir: root})
	p, ok := r.Get("imap-mcp")
	if !ok {
		t.Fatal("service plugin not discovered")
	}
	if pluginDeclares(p, HookPostSessionOutput) {
		t.Fatal("service plugin must not declare subprocess hooks")
	}
	resp, err := r.Fanout(context.Background(), HookPostSessionOutput, Request{Line: "orig"})
	if err != nil || resp.Action != "pass" || resp.Line != "orig" {
		t.Fatalf("Fanout should skip service plugin: resp=%+v err=%v", resp, err)
	}
	if _, err := r.Invoke(context.Background(), "imap-mcp", HookPostSessionOutput, Request{}); err == nil ||
		!strings.Contains(err.Error(), "cannot be invoked") {
		t.Fatalf("Invoke on service plugin should error, got %v", err)
	}
	if p.InvokeCount != 0 {
		t.Fatalf("service plugin must not have been executed, InvokeCount=%d", p.InvokeCount)
	}
}

func TestRegistry_Install_RejectsInvalidManifest(t *testing.T) {
	src := t.TempDir()
	writeManifest(t, src, "name: bad-svc\nkind: service\nconnection:\n  base_url: http://h\n  auth:\n    token: literal\n")
	pdir := filepath.Join(t.TempDir(), "plugins")
	r, _ := NewRegistry(Config{Enabled: true, Dir: pdir})
	err := r.Install(src)
	if err == nil || !strings.Contains(err.Error(), "invalid manifest:") {
		t.Fatalf("expected invalid manifest error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(pdir, "bad-svc")); !os.IsNotExist(statErr) {
		t.Fatalf("nothing should be copied on invalid manifest (stat err=%v)", statErr)
	}
}

func TestRegistry_Install_ServiceManifest(t *testing.T) {
	src := t.TempDir()
	writeManifest(t, src, validService)
	pdir := filepath.Join(t.TempDir(), "plugins")
	r, _ := NewRegistry(Config{Enabled: true, Dir: pdir})
	if err := r.Install(src); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if p, ok := r.Get("imap-mcp"); !ok || p.Kind != KindService {
		t.Fatalf("installed service plugin not discovered: %+v", p)
	}
}
