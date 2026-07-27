package scope

import "testing"

func TestExpand(t *testing.T) {
	cases := []struct {
		name  string
		scope map[string]any
		want  int
	}{
		{"single ip", map[string]any{"targets": []any{"1.2.3.4"}}, 1},
		{"small cidr", map[string]any{"targets": []any{"192.168.1.0/30"}}, 2},
		{"domain", map[string]any{"targets": []any{"example.com"}}, 1},
		{"mixed", map[string]any{"targets": []any{"10.0.0.1", "10.0.0.0/30", "a.test"}}, 3},
	}
	for _, c := range cases {
		got, err := Expand(c.scope)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != c.want {
			t.Errorf("%s: got %d targets, want %d (%v)", c.name, len(got), c.want, got)
		}
	}
}
