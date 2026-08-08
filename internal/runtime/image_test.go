package runtime

import "testing"

func TestImageVersion(t *testing.T) {
	cases := []struct {
		image string
		want  string
	}{
		{"cylism-nanobot-runtime:0.3.0", "0.3.0"},
		{"example/nanobot:v1.2.3", "v1.2.3"},
		{"ghcr.io/org/app:2026-08-08", "2026-08-08"},
		{"example/nanobot:latest", ""},
		{"example/nanobot@sha256:abc123", ""},
		{"example/nanobot", ""},
		{"registry:5000/nanobot", ""},
		{"", ""},
		{"  example/nanobot:1.0  ", "1.0"},
	}
	for _, tc := range cases {
		if got := ImageVersion(tc.image); got != tc.want {
			t.Errorf("ImageVersion(%q) = %q, want %q", tc.image, got, tc.want)
		}
	}
}
