package config

import (
	"testing"
)

func TestValidate_DevNoAuthRequiresLoopback(t *testing.T) {
	base := func() Config {
		c := Default()
		c.Supervisor = "direct" // always available, so the test is host-independent
		return c
	}

	tests := []struct {
		name    string
		listen  string
		wantErr bool
	}{
		{"loopback ip", "127.0.0.1:8080", false},
		{"localhost", "localhost:8080", false},
		{"all interfaces", "0.0.0.0:8080", true},
		{"lan ip", "192.168.1.10:8080", true},
		{"empty", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			c.DevNoAuth = true
			c.Listen = tc.listen
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("dev_no_auth on %q must be rejected", tc.listen)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("dev_no_auth on %q must be accepted, got %v", tc.listen, err)
			}
		})
	}

	// Without dev_no_auth, a non-loopback listen is fine.
	c := base()
	c.Listen = "0.0.0.0:8080"
	if err := c.Validate(); err != nil {
		t.Fatalf("non-loopback listen without dev_no_auth must be accepted, got %v", err)
	}
}

func TestValidate_SupervisorAndLogLevel(t *testing.T) {
	c := Default()
	c.Supervisor = "k8s"
	if err := c.Validate(); err == nil {
		t.Fatal("an unknown supervisor must be rejected")
	}

	c = Default()
	c.Supervisor = "direct"
	c.LogLevel = "verbose"
	if err := c.Validate(); err == nil {
		t.Fatal("an unknown log level must be rejected")
	}

	c = Default()
	c.Supervisor = "direct"
	c.LogLevel = "WARN" // case-insensitive
	if err := c.Validate(); err != nil {
		t.Fatalf("WARN log level must be accepted, got %v", err)
	}
}

func TestApplyEnv_OverridesAndMalformedBool(t *testing.T) {
	t.Setenv("VALHEIM_UI_LISTEN", "0.0.0.0:9000")
	t.Setenv("VALHEIM_UI_DEV_NO_AUTH", "true")
	c := Default()
	applyEnv(&c)
	if c.Listen != "0.0.0.0:9000" {
		t.Fatalf("env must override listen, got %q", c.Listen)
	}
	if !c.DevNoAuth {
		t.Fatal("env must set dev_no_auth")
	}

	// A malformed boolean keeps the prior value rather than flipping or erroring.
	c2 := Default()
	c2.DevNoAuth = true
	t.Setenv("VALHEIM_UI_DEV_NO_AUTH", "ture")
	applyEnv(&c2)
	if !c2.DevNoAuth {
		t.Fatal("a malformed boolean env var must leave the existing value unchanged")
	}
}
