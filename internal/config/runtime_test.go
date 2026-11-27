package config

import (
	"slices"
	"strings"
	"testing"
)

func TestUmaskDefaultAndOverride(t *testing.T) {
	reg := testRegistry(t)
	c, err := Load(inputs(nil, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	if c.Data.Umask != 0o002 {
		t.Fatalf("default umask %o, want 002", c.Data.Umask)
	}
	c, err = Load(inputs([]string{"--data.umask=0027"}, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	if c.Data.Umask != 0o027 {
		t.Fatalf("umask %o, want 027", c.Data.Umask)
	}
	for _, bad := range []string{"8", "abc", "1777", "-1"} {
		if _, err := Load(inputs([]string{"--data.umask=" + bad}, nil, nil), reg); err == nil || !strings.Contains(err.Error(), "octal") {
			t.Errorf("umask %q accepted: %v", bad, err)
		}
	}
}

func TestEgressAllow(t *testing.T) {
	reg := testRegistry(t)
	c, err := Load(inputs(nil, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Server.EgressAllow) != 0 {
		t.Fatalf("default allow list %v, want empty", c.Server.EgressAllow)
	}
	env := []string{"CORSARR_SERVER_EGRESSALLOW_0=api.example.org", "CORSARR_SERVER_EGRESSALLOW_1=*.example.net",
		"CORSARR_SERVER_EGRESSALLOW_2=10.0.0.0/8"}
	c, err = Load(inputs(nil, env, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"api.example.org", "*.example.net", "10.0.0.0/8"}; !slices.Equal(c.Server.EgressAllow, want) {
		t.Fatalf("allow list %v, want %v", c.Server.EgressAllow, want)
	}
	for _, bad := range []string{"http://x.org", "a/b", "*"} {
		if _, err := Load(inputs(nil, []string{"CORSARR_SERVER_EGRESSALLOW_0=" + bad}, nil), reg); err == nil {
			t.Errorf("egress host %q accepted", bad)
		}
	}
}
