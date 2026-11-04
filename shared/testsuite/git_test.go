package testsuite

import (
	"os"
	"testing"
)

func TestScopedGitTrustPreservesConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "user.name")
	t.Setenv("GIT_CONFIG_VALUE_0", "fixture")
	t.Setenv("GIT_CONFIG_KEY_1", "")
	t.Setenv("GIT_CONFIG_VALUE_1", "")
	if err := trustProjectGit(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GIT_CONFIG_COUNT") != "2" || os.Getenv("GIT_CONFIG_KEY_0") != "user.name" || os.Getenv("GIT_CONFIG_VALUE_0") != "fixture" || os.Getenv("GIT_CONFIG_KEY_1") != "safe.directory" || os.Getenv("GIT_CONFIG_VALUE_1") != root {
		t.Fatal("scoped Git configuration was not appended")
	}
	for _, value := range []string{"invalid", "-1", "1025"} {
		t.Setenv("GIT_CONFIG_COUNT", value)
		if err := trustProjectGit(); err == nil {
			t.Fatalf("accepted invalid Git config count %q", value)
		}
	}
}
