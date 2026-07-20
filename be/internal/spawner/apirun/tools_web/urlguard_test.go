package tools_web

import (
	"net"
	"testing"
)

func TestValidateFetchURLSyntax(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"public host", "https://ok.example.com/path", false},
		{"public ip literal", "https://1.1.1.1", false},
		{"explicit 443", "https://ok.example.com:443/x", false},
		{"http scheme", "http://ok.example.com", true},
		{"file scheme", "file:///etc/passwd", true},
		{"userinfo", "https://u:p@ok.example.com", true},
		{"non-443 port", "https://ok.example.com:8080", true},
		{"empty host", "https:///path", true},
		{"empty", "", true},
		{"invalid url", "https://[::1", true},
		{"loopback v4 literal", "https://127.0.0.1", true},
		{"cloud metadata literal", "https://169.254.169.254", true},
		{"private 10 literal", "https://10.0.0.1", true},
		{"private 192.168 literal", "https://192.168.1.1", true},
		{"private 172.16 literal", "https://172.16.0.1", true},
		{"loopback v6 literal", "https://[::1]", true},
		{"ula v6 literal", "https://[fc00::1]", true},
		{"link-local v6 literal", "https://[fe80::1]", true},
		{"unresolved hostname allowed here", "https://not-yet-resolved.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFetchURLSyntax(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateFetchURLSyntax(%q) err=%v, wantErr=%v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestCheckResolvedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"public v4", "1.1.1.1", false},
		{"public v4 other", "8.8.8.8", false},
		{"public v6", "2606:4700:4700::1111", false},
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"link-local v4 (metadata)", "169.254.169.254", true},
		{"link-local v6", "fe80::1", true},
		{"multicast v4", "224.0.0.1", true},
		{"multicast v6", "ff02::1", true},
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"unique local v6 (fc00::/7)", "fc00::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tc.ip)
			}
			err := CheckResolvedIP(ip)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckResolvedIP(%q) err=%v, wantErr=%v", tc.ip, err, tc.wantErr)
			}
		})
	}
}
