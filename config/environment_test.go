package config

import "testing"

func TestConfigEnvironmentFamily(t *testing.T) {
	tests := []struct {
		env    Environment
		isISBE bool
		isDOME bool
	}{
		{ISBE_DEV, true, false},
		{ISBE_PRE, true, false},
		{ISBE_PRO, true, false},
		{DOME_DEV, false, true},
		{DOME_PRE, false, true},
		{DOME_PRO, false, true},
		{LOCAL, false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			c := &Config{Environment: tt.env}
			if got := c.IsISBE(); got != tt.isISBE {
				t.Errorf("IsISBE() = %v, want %v", got, tt.isISBE)
			}
			if got := c.IsDOME(); got != tt.isDOME {
				t.Errorf("IsDOME() = %v, want %v", got, tt.isDOME)
			}
		})
	}
}
