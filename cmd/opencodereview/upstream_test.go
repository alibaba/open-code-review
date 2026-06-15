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

func TestParseSymrefDefaultBranch(t *testing.T) {
	out := "ref: refs/heads/main\tHEAD\n" +
		"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0\tHEAD\n"
	branch, err := parseSymrefDefaultBranch(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("got %q, want main", branch)
	}
}

func TestParseSymrefDefaultBranchMaster(t *testing.T) {
	out := "ref: refs/heads/master\tHEAD\n0000\tHEAD\n"
	branch, err := parseSymrefDefaultBranch(out)
	if err != nil || branch != "master" {
		t.Fatalf("got %q, err %v; want master", branch, err)
	}
}

func TestParseSymrefDefaultBranchMissing(t *testing.T) {
	if _, err := parseSymrefDefaultBranch("0000\tHEAD\n"); err == nil {
		t.Fatal("expected error when no symref line present")
	}
}
