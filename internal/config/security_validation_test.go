package config

import (
	"strings"
	"testing"
)

func TestSecurityValidation(t *testing.T) {
	cases := []struct {
		name    string
		sandbox *SandboxConfig
		field   string // "" = expect no error
	}{
		{"default", nil, ""},
		{"empty type", &SandboxConfig{}, ""},
		{"docker", &SandboxConfig{Type: "docker"}, ""},
		{"local", &SandboxConfig{Type: "local_restricted"}, ""},
		{"unknown", &SandboxConfig{Type: "dockre"}, "type"},
		{"contradictory network", &SandboxConfig{Type: "docker", AllowNetwork: true, NetworkIsolated: true}, "allow_network"},
		{"writable without mount", &SandboxConfig{Type: "docker", MountWorkdirWritable: true}, "mount_workdir_writable"},
		{"negative timeout", &SandboxConfig{ResourceLimits: ResourceLimits{TimeoutSec: -1}}, "resource_limits.timeout_sec"},
		{"negative pids", &SandboxConfig{Type: "docker", ResourceLimits: ResourceLimits{PidsLimit: -2}}, "resource_limits.pids_limit"},
		{"negative output", &SandboxConfig{ResourceLimits: ResourceLimits{MaxOutputBytes: -2}}, "resource_limits.max_output_bytes"},
		{"unlimited", &SandboxConfig{Type: "docker", ResourceLimits: ResourceLimits{PidsLimit: -1, MaxOutputBytes: -1}}, ""},
		{"dash image", &SandboxConfig{Type: "docker", Image: "--privileged"}, "image"},
		{"local image", &SandboxConfig{Type: "local_restricted", Image: "alpine"}, "image"},
		{"local user", &SandboxConfig{User: "1000"}, "user"},
		{"local isolation", &SandboxConfig{Type: "local_restricted", NetworkIsolated: true}, "network_isolated"},
		{"local network", &SandboxConfig{AllowNetwork: true}, "allow_network"},
		{"local mount", &SandboxConfig{MountWorkdir: true}, "mount_workdir"},
		{"local cpu", &SandboxConfig{Type: "local_restricted", ResourceLimits: ResourceLimits{CPULimit: "1"}}, "resource_limits.cpu_limit"},
		{"local memory", &SandboxConfig{Type: "local_restricted", ResourceLimits: ResourceLimits{MemoryLimit: "1g"}}, "resource_limits.memory_limit"},
		{"local pids", &SandboxConfig{ResourceLimits: ResourceLimits{PidsLimit: 10}}, "resource_limits.pids_limit"},
	}
	// The Duration-overflow boundary only exists where int is 64-bit; on a
	// 32-bit target these values don't fit in TimeoutSec's int at all.
	// Runtime conversions through a variable: a constant int(maxTimeoutSec)
	// would itself be a compile-time overflow on 32-bit.
	bound := maxTimeoutSec
	if over := bound + 1; int64(int(over)) == over {
		cases = append(cases,
			struct {
				name    string
				sandbox *SandboxConfig
				field   string
			}{"overflowing timeout", &SandboxConfig{ResourceLimits: ResourceLimits{TimeoutSec: int(over)}}, "resource_limits.timeout_sec"},
			struct {
				name    string
				sandbox *SandboxConfig
				field   string
			}{"max timeout", &SandboxConfig{ResourceLimits: ResourceLimits{TimeoutSec: int(bound)}}, ""},
		)
	}
	for _, tc := range cases {
		for _, inline := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/global", true: "/inline"}[inline], func(t *testing.T) {
				tool := ToolDefinition{Name: "shell", Type: "cli", Sandbox: tc.sandbox}
				cfg := Config{Tools: []ToolDefinition{tool}}
				prefix := "tools[0].sandbox."
				if inline {
					cfg = Config{Agents: []AgentDefinition{{Name: "worker", ToolsInline: []ToolDefinition{tool}}}}
					prefix = "agents[0].tools_inline[0].sandbox."
				}
				errs := cfg.Validate()
				if tc.field == "" {
					if len(errs) != 0 {
						t.Fatalf("unexpected errors: %v", errs)
					}
					return
				}
				for _, e := range errs {
					if e.Field == prefix+tc.field {
						return
					}
				}
				t.Fatalf("missing %s%s: %v", prefix, tc.field, errs)
			})
		}
	}
}

func TestSecurityValidationNegativeToolTimeout(t *testing.T) {
	cfg := Config{Tools: []ToolDefinition{{Name: "shell", Type: "cli", Timeout: -1}}}
	for _, e := range cfg.Validate() {
		if strings.HasSuffix(e.Field, "timeout_seconds") {
			return
		}
	}
	t.Fatal("negative CLI timeout accepted")
}

func TestValidateSandboxDirect(t *testing.T) {
	for _, s := range []*SandboxConfig{nil, {}, {Type: "docker"}, {Type: "local_restricted"}} {
		if err := ValidateSandbox(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []*SandboxConfig{{Type: "typo"}, {Type: "docker", Image: "--privileged"}, {Type: "docker", AllowNetwork: true, NetworkIsolated: true}} {
		if err := ValidateSandbox(s); err == nil {
			t.Fatalf("accepted unsafe sandbox: %+v", s)
		}
	}
}
