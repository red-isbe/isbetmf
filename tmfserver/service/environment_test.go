package service

import (
	"testing"

	"github.com/hesusruiz/isbetmf/config"
)

func TestServiceEnvironmentFamily(t *testing.T) {
	tests := []struct {
		env    config.Environment
		isISBE bool
		isDOME bool
	}{
		{config.ISBE_DEV, true, false},
		{config.ISBE_PRE, true, false},
		{config.ISBE_PRO, true, false},
		{config.DOME_DEV, false, true},
		{config.DOME_PRE, false, true},
		{config.DOME_PRO, false, true},
		{config.LOCAL, false, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			svc := &Service{environment: tt.env}
			if got := svc.IsISBE(); got != tt.isISBE {
				t.Errorf("IsISBE() = %v, want %v", got, tt.isISBE)
			}
			if got := svc.IsDOME(); got != tt.isDOME {
				t.Errorf("IsDOME() = %v, want %v", got, tt.isDOME)
			}
		})
	}
}
