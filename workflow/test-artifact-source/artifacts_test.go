package main

import "testing"

func TestPartialRetrySelection(t *testing.T) {
	all := []artifact{{1, "oci-a-amd64-42-1", false}, {2, "oci-a-arm64-42-1", false}, {3, "oci-a-arm64-42-2", false}}
	ids, err := selectArtifacts(all, []string{"a-amd64", "a-arm64"}, "42", 2)
	if err != nil || len(ids) != 2 || ids[0] != "1" || ids[1] != "3" {
		t.Fatalf("%v %v", ids, err)
	}
	for name, items := range map[string][]artifact{
		"missing":        all[1:],
		"duplicate":      append(append([]artifact{}, all...), artifact{4, "oci-a-arm64-42-2", false}),
		"expired latest": append(append([]artifact{}, all...), artifact{4, "oci-a-arm64-42", true}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := selectArtifacts(items, []string{"a-amd64", "a-arm64"}, "42", 2); err == nil {
				t.Fatal("incoherent set accepted")
			}
		})
	}
	all = append(all, artifact{5, "oci-a-arm64-42", false})
	ids, err = selectArtifacts(all, []string{"a-amd64", "a-arm64"}, "42", 2)
	if err != nil || ids[1] != "5" {
		t.Fatalf("%v %v", ids, err)
	}
}
