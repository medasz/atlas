package scan

import "testing"

func TestParsePortSpec(t *testing.T) {
	cases := []struct {
		spec string
		want int
	}{
		{"80,443", 2},
		{"80-83", 4},
		{"80,443,8080-8081", 4},
		{"1-1000", 1000},
	}
	for _, c := range cases {
		got, err := ParsePortSpec(c.spec)
		if err != nil {
			t.Fatalf("%s: %v", c.spec, err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d, want %d", c.spec, len(got), c.want)
		}
	}
	if _, err := ParsePortSpec("80-"); err == nil {
		t.Error("expected error for invalid range")
	}
}

func TestGuessService(t *testing.T) {
	if guessService(80, "") != "http" {
		t.Error("port 80 should be http")
	}
	if guessService(3306, "") != "mysql" {
		t.Error("port 3306 should be mysql")
	}
	if guessService(9999, "HTTP/1.1 200 OK") != "http" {
		t.Error("http banner should guess http")
	}
}
