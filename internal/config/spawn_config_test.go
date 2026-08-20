package config

import (
	"testing"
	"time"
)

func TestSpawnConfigDefaults(t *testing.T) {
	var zero SpawnConfig
	if got := zero.EffectiveMaxConcurrent(); got != 4 {
		t.Errorf("EffectiveMaxConcurrent zero = %d, want 4", got)
	}
	if got := zero.EffectiveMaxDepth(); got != 1 {
		t.Errorf("EffectiveMaxDepth zero = %d, want 1", got)
	}
	if got := zero.EffectiveTimeout(); got != 300*time.Second {
		t.Errorf("EffectiveTimeout zero = %v, want 300s", got)
	}
	set := SpawnConfig{MaxConcurrent: 2, MaxDepth: 3, TimeoutSeconds: 10}
	if set.EffectiveMaxConcurrent() != 2 || set.EffectiveMaxDepth() != 3 || set.EffectiveTimeout() != 10*time.Second {
		t.Errorf("explicit values not honored: %+v", set)
	}
}
