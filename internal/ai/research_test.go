package ai_test

import (
	"net"
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/ai"
)

func TestIsPrivateOrReservedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"IPv4 Loopback", "127.0.0.1", true},
		{"IPv4 Loopback arbitrary", "127.123.45.67", true},
		{"IPv4 Private 10.x", "10.0.0.1", true},
		{"IPv4 Private 192.168.x", "192.168.1.100", true},
		{"IPv4 Private 172.16.x", "172.16.0.1", true},
		{"IPv4 Private 172.31.x", "172.31.255.255", true},
		{"Carrier-Grade NAT", "100.64.0.1", true},
		{"Link-Local", "169.254.1.1", true},
		{"Cloud Metadata IP", "169.254.169.254", true},
		{"Current Network", "0.0.0.0", true},
		{"Test Net 1", "192.0.2.1", true},
		{"Test Net 2", "198.51.100.1", true},
		{"Test Net 3", "203.0.113.1", true},
		{"Multicast", "224.0.0.1", true},
		{"Reserved 240.x", "240.0.0.1", true},
		{"IPv6 Loopback", "::1", true},
		{"IPv6 Unspecified", "::", true},
		{"IPv6 Link-Local", "fe80::1", true},
		{"IPv6 Unique-Local", "fc00::1", true},
		{"IPv6 Multicast", "ff02::1", true},
		{"IPv4-mapped IPv6 loopback", "::ffff:127.0.0.1", true},
		{"IPv4-mapped IPv6 private", "::ffff:192.168.1.1", true},
		{"IPv4-mapped IPv6 metadata", "::ffff:169.254.169.254", true},
		{"Public IPv4 Google DNS", "8.8.8.8", false},
		{"Public IPv4 Cloudflare DNS", "1.1.1.1", false},
		{"Public IPv6 Cloudflare", "2606:4700:4700::1111", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %s", tt.ip)
			}
			got := ai.IsPrivateOrReservedIP(ip)
			if got != tt.blocked {
				t.Errorf("IP %s: expected blocked=%v, got %v", tt.ip, tt.blocked, got)
			}
		})
	}
}
