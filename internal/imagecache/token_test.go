package imagecache

import (
	"strings"
	"testing"
)

func TestTokensRoundTripAndRefuseTampering(t *testing.T) {
	s := NewSigner([]byte("secret"))
	origin := "https://image.example/t/p/w500/abc.jpg"
	tok := s.Sign(RoleBackdrop, origin)
	if role, got, ok := s.Open(tok); !ok || role != RoleBackdrop || got != origin {
		t.Fatal(role, got, ok)
	}
	if _, _, ok := NewSigner([]byte("other")).Open(tok); ok {
		t.Fatal("foreign key accepted")
	}
	flipped := []byte(tok)
	flipped[len(flipped)-1] ^= 1
	for _, bad := range []string{"", "!!!", "AAAA", string(flipped), strings.Repeat("A", 40)} {
		if _, _, ok := s.Open(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, _, ok := s.Open(s.Sign(Role('x'), origin)); ok {
		t.Fatal("unknown role accepted")
	}
}

func TestLinks(t *testing.T) {
	s := NewSigner([]byte("secret"))
	if got := s.Link(RolePoster, "https://img.example/a.jpg"); !strings.HasPrefix(got, LinkPrefix) || len(got) < len(LinkPrefix)+40 {
		t.Fatal(got)
	}
	for _, none := range []string{"", "file:///etc/passwd", "javascript:alert(1)", "https://", "not a url"} {
		if got := s.Link(RolePoster, none); got != "" {
			t.Errorf("%q linked as %q", none, got)
		}
	}
	if Key("https://a/x") == Key("https://a/y") || len(Key("x")) != 64 {
		t.Fatal("keys")
	}
	if roleName(RolePoster) != "poster" || roleName(RoleBackdrop) != "backdrop" {
		t.Fatal("role names")
	}
}
