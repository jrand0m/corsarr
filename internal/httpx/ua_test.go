package httpx

import "testing"

func TestUserAgentKeepsTheProductTokenAndAppendsAPluginComment(t *testing.T) {
	cases := map[string]string{
		"":                            "corsarr/1.2.3",
		"( https://example.invalid )": "corsarr/1.2.3 ( https://example.invalid )",
		"custom-agent/1":              "custom-agent/1",
	}
	for set, want := range cases {
		if got := userAgent(set, "1.2.3"); got != want {
			t.Errorf("userAgent(%q) = %q, want %q", set, got, want)
		}
	}
}
