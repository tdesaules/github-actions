package main

import "testing"

func TestSelection(t *testing.T) {
	components := []component{{"lint/a", "lint-a", "example/a@sha256:abc"}, {"lint/b", "lint-b", "example/b@sha256:def"}}
	for _, tc := range []struct {
		path    string
		count   int
		rebuild bool
	}{
		{"README.md", 0, false}, {"lint/a/Containerfile", 2, true}, {"lint/a/action.yml", 2, false}, {"shared/runtime/main.go", 4, true}, {".github/workflows/main.yml", 4, false}, {"lint/deleted/Containerfile", 0, false},
	} {
		m := selectImages(components, []string{tc.path}, false)
		if len(m.Include) != tc.count {
			t.Fatalf("%s: %+v", tc.path, m)
		}
		for _, job := range m.Include {
			if job.Rebuild != tc.rebuild {
				t.Fatalf("%s: %+v", tc.path, job)
			}
		}
	}
	if len(selectImages(components, nil, true).Include) != 4 {
		t.Fatal("full matrix")
	}
	components[0].reference = "example/a:unpublished"
	if !selectImages(components, []string{"lint/a/action.yml"}, false).Include[0].Rebuild {
		t.Fatal("unpublished image reused")
	}
}
