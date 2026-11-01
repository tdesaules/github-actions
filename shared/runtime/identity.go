package runtimeaction

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IdentityTokenFile passes Cosign a private file path rather than a JWT in argv.
// Cosign 3.1 accepts a token file for --identity-token. Keep it outside workspace
// report globs and remove it after the signer exits, including on failure.
func IdentityTokenFile(token string) (string, func(), error) {
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", nil, fmt.Errorf("invalid identity token")
	}
	directory, err := os.MkdirTemp("", "cosign-identity-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	path := filepath.Join(directory, "token")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}
