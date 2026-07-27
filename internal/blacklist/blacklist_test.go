package blacklist

import (
	"testing"

	"atlas/internal/model"
)

func TestMatchEntries(t *testing.T) {
	entries := []model.BlacklistItem{
		{Type: "ip", Value: "10.0.0.5"},
		{Type: "cidr", Value: "192.168.0.0/16"},
		{Type: "domain", Value: "example.com"},
	}
	cases := []struct {
		target string
		want   bool
	}{
		{"10.0.0.5", true},
		{"10.0.0.6", false},
		{"192.168.1.1", true},
		{"172.16.0.1", false},
		{"api.example.com", true},
		{"example.org", false},
		{"example.com", true},
	}
	for _, c := range cases {
		if got := matchEntries(c.target, entries); got != c.want {
			t.Errorf("matchEntries(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}
