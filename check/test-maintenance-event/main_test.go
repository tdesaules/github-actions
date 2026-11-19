package main

import "testing"

func TestEventPolicy(t *testing.T) {
	for _, test := range []struct {
		ref, event string
		want       bool
	}{
		{"refs/heads/main", "schedule", true},
		{"refs/heads/main", "workflow_dispatch", true},
		{"refs/heads/other", "workflow_dispatch", false},
		{"refs/tags/1.0.0", "push", false},
		{"refs/heads/main", "push", false},
	} {
		if valid(test.ref, test.event) != test.want {
			t.Errorf("unexpected routing: %+v", test)
		}
	}
}
