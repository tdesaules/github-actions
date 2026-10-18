package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, path, data string) {
	t.Helper()
	file := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"oci/export-image", "oci/import-image", "check/test-yaml"} {
		family := filepath.Base(filepath.Dir(directory))
		write(t, root, directory+"/action.yml", "runs:\n  using: docker\n  image: docker://example.invalid/"+family+"-"+filepath.Base(directory)+":unpublished\n")
		write(t, root, directory+"/Containerfile", "FROM scratch\nCOPY go.mod ./\nCOPY shared/"+family+"/ ./shared/\nCOPY "+directory+"/main.go /entrypoint\n")
		write(t, root, directory+"/main.go", "package main\n")
	}
	write(t, root, "workflow/test-images/action.yml", "runs:\n  using: composite\n  steps:\n    - uses: ./oci/export-image\n")
	write(t, root, "shared/oci/entrypoint.go", "package runtime\n")
	write(t, root, "shared/check/entrypoint.go", "package check\n")
	write(t, root, "go.mod", "module fixture\n")
	return root
}

func TestDocumentationSelectsOnlyMarkdownAndLinks(t *testing.T) {
	root := fixture(t)
	write(t, root, "oci/export-image/README.md", "# Export\n")
	result, err := Select(root, []string{"oci/export-image/README.md"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matrix.Include) != 0 || len(result.Actions) != 0 || len(result.GoPackages) != 0 || result.OrchestrationTests {
		t.Fatalf("documentation selected unrelated work: %+v", result)
	}
	if !reflect.DeepEqual(result.Checks, map[string][]string{"markdown": {"oci/export-image/README.md"}, "links": {"oci/export-image/README.md"}}) {
		t.Fatal(result.Checks)
	}
}

func TestContainerfileSelectsOneImageAndCompositeDependents(t *testing.T) {
	result, err := Select(fixture(t), []string{"oci/export-image/Containerfile"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matrix.Include) != 2 {
		t.Fatal(result.Matrix)
	}
	for _, job := range result.Matrix.Include {
		if job.Image != "oci-export-image" || !job.Rebuild {
			t.Fatal(job)
		}
	}
	if !reflect.DeepEqual(result.Actions, []string{"oci/export-image", "workflow/test-images"}) {
		t.Fatal(result.Actions)
	}
}

func TestMetadataDoesNotRebuildAndSharedSourcesAreScoped(t *testing.T) {
	root := fixture(t)
	metadata, err := Select(root, []string{"oci/export-image/action.yml"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range metadata.Matrix.Include {
		if job.Rebuild {
			t.Fatal("metadata rebuilt image")
		}
	}
	shared, err := Select(root, []string{"shared/oci/entrypoint.go"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared.Matrix.Include) != 4 {
		t.Fatal(shared.Matrix)
	}
	for _, job := range shared.Matrix.Include {
		if job.Image == "check-test-yaml" || !job.Rebuild {
			t.Fatal(job)
		}
	}
}

func TestDeletedSourcesAndRenamesRetainAffectedImages(t *testing.T) {
	root := fixture(t)
	if err := os.Remove(filepath.Join(root, "oci/export-image/main.go")); err != nil {
		t.Fatal(err)
	}
	result, err := Select(root, []string{"oci/export-image/main.go", "oci/import-image/main.go"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matrix.Include) != 4 || !reflect.DeepEqual(result.Deleted, []string{"oci/export-image/main.go"}) {
		t.Fatal(result)
	}
	removed, err := Select(root, []string{"oci/deleted/main.go"}, false)
	if err != nil || len(removed.Matrix.Include) != 0 {
		t.Fatal(removed, err)
	}
}

func TestExtensionlessShellAndSpaceContainingPaths(t *testing.T) {
	root := fixture(t)
	write(t, root, "scripts/my script", "#!/usr/bin/env bash\nprintf '%s\\n' hello\n")
	write(t, root, "scripts/recipe.script", "#!/bin/sh\nexit 0\n")
	result, err := Select(root, []string{"scripts/my script", "scripts/recipe.script"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks["shell"]) != 2 || len(result.Matrix.Include) != 0 {
		t.Fatal(result)
	}
}

func TestCopyParserStagesJSONWildcardsAndVariables(t *testing.T) {
	sources, err := copySources("COPY --from=tool /binary /bin/tool\nCOPY --chown=1:1 [\"source with spaces\", \"/dst\"]\nCOPY shared/*.go \\\n /src/\nCOPY $SOURCE /destination\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sources, []string{"source with spaces", "shared/*.go", "."}) {
		t.Fatal(sources)
	}
	if !dependency("shared/*", "shared/runtime/main.go") || dependency("shared/check", "shared/runtime/main.go") {
		t.Fatal("dependency matching incorrect")
	}
}

func TestEmptyAndFullPlans(t *testing.T) {
	root := fixture(t)
	empty, err := Select(root, nil, false)
	if err != nil || len(empty.Matrix.Include) != 0 || empty.Matrix.Include == nil {
		t.Fatal(empty, err)
	}
	full, err := Select(root, nil, true)
	if err != nil || len(full.Matrix.Include) != 6 {
		t.Fatal(full, err)
	}
	if !reflect.DeepEqual(full.GoPackages, []string{"./..."}) {
		t.Fatal(full.GoPackages)
	}
}

func TestDeletedCheckConfigurationSelectsDependentDocuments(t *testing.T) {
	root := fixture(t)
	write(t, root, "README.md", "# Fixture\n")
	result, err := Select(root, []string{"lychee.toml", "README.md"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Checks["links"], []string{"README.md"}) || len(result.Matrix.Include) != 0 {
		t.Fatal(result)
	}
}

func TestRunnerFixtureIsNotACatalogueAction(t *testing.T) {
	root := fixture(t)
	write(t, root, "tests/runner/action.yml", "runs:\n  using: docker\n  image: Dockerfile\n")
	result, err := Select(root, []string{"tests/runner/action.yml"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checks["action-metadata"]) != 0 || len(result.Checks["yaml"]) != 1 || !result.OrchestrationTests {
		t.Fatal(result)
	}
}
