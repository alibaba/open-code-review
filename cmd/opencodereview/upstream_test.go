package main

import "testing"

func TestIsUpstreamURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://github.com/o/r", true},
		{"git://example.com/r.git", true},
		{"ssh://git@example.com/r.git", true},
		{"file:///tmp/bare.git", true},
		{"git@github.com:o/r.git", true},
		{"/abs/path/bare.git", true},
		{"./rel/bare.git", true},
		{"../rel/bare.git", true},
		{"upstream", false},
		{"origin", false},
		{"my-remote", false},
	}
	for _, c := range cases {
		if got := isUpstreamURL(c.in); got != c.want {
			t.Errorf("isUpstreamURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
