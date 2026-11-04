package testsuite

import (
	"fmt"
	"os"
	"strconv"
)

// Trust only the explicitly selected checkout inside the action. Docker's UID
// need not match the runner's checkout owner. Do not write global Git config or
// replace authentication/config entries already supplied through the environment.
func trustProjectGit() error {
	count := 0
	if value := os.Getenv("GIT_CONFIG_COUNT"); value != "" {
		var err error
		count, err = strconv.Atoi(value)
		if err != nil || count < 0 || count > 1024 {
			return fmt.Errorf("invalid GIT_CONFIG_COUNT")
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	for name, value := range map[string]string{
		fmt.Sprintf("GIT_CONFIG_KEY_%d", count):   "safe.directory",
		fmt.Sprintf("GIT_CONFIG_VALUE_%d", count): root,
		"GIT_CONFIG_COUNT":                        strconv.Itoa(count + 1),
	} {
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	return nil
}
