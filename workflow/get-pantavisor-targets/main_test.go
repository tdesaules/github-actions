package main

import "testing"

func TestSelection(t *testing.T) {
	for _, project := range []string{"core", "k0s", "system"} {
		for _, tc := range []struct {
			name  string
			files []string
			full  bool
			want  selection
		}{
			{"empty", []string{}, false, selection{}},
			{"documentation", []string{"README.md", "docs/components.md"}, false, selection{}},
			{"tests only", []string{"tests/test_package.py"}, false, selection{Check: true}},
			{"service without extension", []string{"services/core-init"}, false, selection{true, true, true}},
			{"recipe", []string{"build/meta-core/recipes-core/images/image.bb"}, false, selection{true, true, true}},
			{"composition", []string{"compositions/profiles/k0s.json"}, false, selection{true, true, true}},
			{"deleted or renamed input", []string{"services/old-name", "services/new-name"}, false, selection{true, true, true}},
			{"license", []string{"LICENSE"}, false, selection{true, true, true}},
			{"new branch or release", []string{}, true, selection{true, true, true}},
			{"release with docs diff", []string{"README.md"}, true, selection{true, true, true}},
			{"workflow", []string{".github/workflows/main.yml"}, false, selection{true, true, true}},
			{"spaces", []string{"services/my service"}, false, selection{true, true, true}},
		} {
			t.Run(project+"/"+tc.name, func(t *testing.T) {
				got, err := selectTargets(project, tc.files, tc.full)
				if err != nil || got != tc.want {
					t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
				}
			})
		}
	}
}

func TestInvalidInputs(t *testing.T) {
	if _, err := selectTargets("unknown", nil, false); err == nil {
		t.Fatal("accepted unknown project")
	}
	for _, file := range []string{"", ".", "..", "../outside", "/absolute", "a/../b", "a\\b", "a\noutput=false", "a\x00b"} {
		if _, err := selectTargets("core", []string{file}, true); err == nil {
			t.Errorf("accepted %q", file)
		}
	}
}
