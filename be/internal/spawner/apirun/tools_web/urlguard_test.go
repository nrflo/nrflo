package tools_web

import (
	"fmt"
	"net"
	"testing"
)

func TestValidateFetchURL(t *testing.T) {
	orig := lookupIP
	defer func() { lookupIP = orig }()
	lookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "ok.example.com":
			return []net.IP{net.ParseIP("1.1.1.1")}, nil
		case "rebind.example.com": // hostname that resolves to cloud metadata
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		case "nxdomain.example.com":
			return nil, fmt.Errorf("no such host")
		}
		return nil, fmt.Errorf("unexpected host %q", host)
	}

	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public ip literal", "https://1.1.1.1", false},
		{"public host", "https://ok.example.com/path", false},
		{"explicit 443", "https://ok.example.com:443/x", false},
		{"http scheme", "http://ok.example.com", true},
		{"file scheme", "file:///etc/passwd", true},
		{"userinfo", "https://u:p@ok.example.com", true},
		{"non-443 port", "https://ok.example.com:8080", true},
		{"loopback v4", "https://127.0.0.1", true},
		{"cloud metadata", "https://169.254.169.254", true},
		{"private 10", "https://10.0.0.1", true},
		{"private 192.168", "https://192.168.1.1", true},
		{"private 172.16", "https://172.16.0.1", true},
		{"loopback v6", "https://[::1]", true},
		{"ula v6", "https://[fc00::1]", true},
		{"link-local v6", "https://[fe80::1]", true},
		{"dns rebind to metadata", "https://rebind.example.com", true},
		{"nxdomain", "https://nxdomain.example.com", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFetchURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateFetchURL(%q) err=%v, wantErr=%v", tc.url, err, tc.wantErr)
			}
		})
	}
}
