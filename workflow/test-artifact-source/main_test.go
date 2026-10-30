package main

import "testing"

func TestExactSourceValidation(t *testing.T) {
	good := sourceRun{"abc", "main", ".github/workflows/main.yml", "push", "success", 1, "completed"}
	if err := validate(good, "abc", "abc"); err != nil {
		t.Fatal(err)
	}
	if validate(good, "abc", "different-checkout") == nil {
		t.Fatal("artifact set derived from a different checkout accepted")
	}
	for _, mutate := range []func(*sourceRun){
		func(r *sourceRun) { r.Revision = "advanced" }, func(r *sourceRun) { r.Branch = "other" }, func(r *sourceRun) { r.Path = "other.yml" }, func(r *sourceRun) { r.Event = "pull_request" }, func(r *sourceRun) { r.Conclusion = "failure" }, func(r *sourceRun) { r.Attempt = 0 }, func(r *sourceRun) { r.Status = "queued" }, func(r *sourceRun) { r.Status = "in_progress" },
	} {
		bad := good
		mutate(&bad)
		if validate(bad, "abc", "abc") == nil {
			t.Fatal("unsafe artifact source accepted", bad)
		}
	}
}
