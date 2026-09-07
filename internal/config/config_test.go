package config

import "testing"

// TestConfig_Validate covers the port-range checks Load now enforces: a
// malformed range (inverted, or half-specified) must be rejected clearly
// rather than left to produce undefined behavior later in
// internal/portpool.
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"both zero (let the OS pick)", Config{}, false},
		{"valid single-port range", Config{PortRangeStart: 42069, PortRangeEnd: 42069}, false},
		{"valid multi-port range", Config{PortRangeStart: 6000, PortRangeEnd: 6010}, false},
		{"inverted range", Config{PortRangeStart: 6010, PortRangeEnd: 6000}, true},
		{"start set, end zero", Config{PortRangeStart: 6000}, true},
		{"end set, start zero", Config{PortRangeEnd: 6000}, true},
		{"negative start", Config{PortRangeStart: -1, PortRangeEnd: 6000}, true},
		{"negative end", Config{PortRangeStart: 6000, PortRangeEnd: -1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want an error for %+v", tt.cfg)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil for %+v", err, tt.cfg)
			}
		})
	}
}

// TestLoad_RejectsInvertedPortRange confirms the check is actually wired
// into Load, not just Validate in isolation: a config.toml with
// port_range_start > port_range_end must fail to load with a clear error.
func TestLoad_RejectsInvertedPortRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := Default()
	cfg.PortRangeStart = 6010
	cfg.PortRangeEnd = 6000
	if err := Save(&cfg); err != nil {
		t.Fatalf("save invalid config: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded on a config with port_range_start > port_range_end, want an error")
	}
}
