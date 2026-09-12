package config

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// maxTimeoutSec is the largest resource_limits.timeout_sec that still fits
// in a time.Duration once multiplied by time.Second. Kept as int64 — as an
// int it would not compile on 32-bit targets (constant overflow); there the
// comparison below simply never triggers, since an int can't reach it.
const maxTimeoutSec int64 = math.MaxInt64 / int64(time.Second)

// validateToolSecurity rejects sandbox options that would otherwise silently
// lose their security meaning: an unknown type, options that only apply to
// the docker sandbox set on a local tool, and contradictory or negative
// settings. An empty type means local_restricted (the default).
func (c *Config) validateToolSecurity(add func(string, string)) {
	validate := func(tool ToolDefinition, path string) {
		if tool.Type == "cli" && tool.Timeout < 0 {
			add(path+".timeout_seconds", "must be non-negative")
		}
		s := tool.Sandbox
		if s == nil {
			return
		}
		path += ".sandbox."
		if strings.HasPrefix(strings.TrimSpace(s.Image), "-") {
			add(path+"image", "must not begin with a dash")
		}
		switch s.Type {
		case "", "docker", "local_restricted":
		default:
			add(path+"type", fmt.Sprintf("unknown sandbox type %q: must be docker or local_restricted (empty defaults to local_restricted)", s.Type))
		}
		if s.AllowNetwork && s.NetworkIsolated {
			add(path+"allow_network", "cannot be combined with network_isolated")
		}
		if s.MountWorkdirWritable && !s.MountWorkdir {
			add(path+"mount_workdir_writable", "requires mount_workdir")
		}
		if s.ResourceLimits.TimeoutSec < 0 {
			add(path+"resource_limits.timeout_sec", "must be non-negative")
		} else if int64(s.ResourceLimits.TimeoutSec) > maxTimeoutSec {
			// Beyond this, seconds * time.Second overflows time.Duration into a
			// negative value and context.WithTimeout cancels immediately.
			add(path+"resource_limits.timeout_sec", fmt.Sprintf("must be at most %d", maxTimeoutSec))
		}
		if s.ResourceLimits.PidsLimit < -1 {
			add(path+"resource_limits.pids_limit", "must be -1 or non-negative")
		}
		if s.ResourceLimits.MaxOutputBytes < -1 {
			add(path+"resource_limits.max_output_bytes", "must be -1 or non-negative")
		}
		if s.Type == "docker" {
			return
		}
		// Docker-only options on a local tool were silently ignored before,
		// which reads as isolation the tool never had.
		for _, option := range []struct {
			name string
			set  bool
		}{
			{"image", s.Image != ""}, {"user", s.User != ""},
			{"mount_workdir", s.MountWorkdir}, {"mount_workdir_writable", s.MountWorkdirWritable},
			{"allow_network", s.AllowNetwork}, {"network_isolated", s.NetworkIsolated},
			{"resource_limits.cpu_limit", s.ResourceLimits.CPULimit != ""},
			{"resource_limits.memory_limit", s.ResourceLimits.MemoryLimit != ""},
			{"resource_limits.pids_limit", s.ResourceLimits.PidsLimit != 0},
		} {
			if option.set {
				add(path+option.name, "requires sandbox type docker")
			}
		}
	}
	for i, t := range c.Tools {
		validate(t, fmt.Sprintf("tools[%d]", i))
	}
	for ai, a := range c.Agents {
		for ti, t := range a.ToolsInline {
			validate(t, fmt.Sprintf("agents[%d].tools_inline[%d]", ai, ti))
		}
	}
}

// ValidateSandbox applies the same rules to a single sandbox before runtime
// use, for callers that construct tools directly without loading a Config.
func ValidateSandbox(s *SandboxConfig) error {
	c := Config{Tools: []ToolDefinition{{Type: "cli", Sandbox: s}}}
	var messages []string
	c.validateToolSecurity(func(field, message string) {
		messages = append(messages, strings.TrimPrefix(field, "tools[0].")+": "+message)
	})
	if len(messages) > 0 {
		return fmt.Errorf("invalid sandbox: %s", strings.Join(messages, "; "))
	}
	return nil
}
