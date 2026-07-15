package cli

import "testing"

func TestIsLocalServer(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://127.0.0.1:6587", true},
		{"http://localhost:6587", true},
		{"http://[::1]:6587", true},
		{"http://0.0.0.0:6587", true},
		{"https://127.0.0.1", true},
		{"http://192.168.1.10:6587", false},
		{"https://nrflo.example.com", false},
		{"http://10.0.0.5:6587", false},
		{"", false},
		{"://bad", false},
	}
	for _, tc := range cases {
		if got := isLocalServer(tc.url); got != tc.want {
			t.Errorf("isLocalServer(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}
