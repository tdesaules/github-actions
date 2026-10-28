package main

import "testing"

func TestExactSourceValidation(t *testing.T) {
	good := sourceRun{"abc", "main", ".github/workflows/main.yml", "push", "success", 1, "completed"}
	if err := validate(good, "abc"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*sourceRun){
		func(r *sourceRun) { r.Revision = "advanced" }, func(r *sourceRun) { r.Branch = "other" }, func(r *sourceRun) { r.Path = "other.yml" }, func(r *sourceRun) { r.Event = "pull_request" }, func(r *sourceRun) { r.Conclusion = "failure" }, func(r *sourceRun) { r.Attempt = 0 },
	} {
		bad := good
		mutate(&bad)
		if validate(bad, "abc") == nil {
			t.Fatal("unsafe artifact source accepted", bad)
		}
	}
}
