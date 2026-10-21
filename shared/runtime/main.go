// Package runtimeaction implements shared machinery for specialized OCI actions.
// No shell is required in the final images. Inputs are never evaluated as code.
package runtimeaction

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

var tool = "unset" // Fixed by the action-specific executable.
var arguments = map[string]string{}
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func input(name, fallback string) string {
	if value, present := arguments[name]; present {
		return value
	}
	if value := os.Getenv("INPUT_" + strings.ToUpper(name)); value != "" {
		return value
	}
	return fallback
}

func required(name string) (string, error) {
	v := input(name, "")
	if v == "" {
		return "", fmt.Errorf("input %q is required", name)
	}
	if strings.ContainsAny(v, "\r\n\x00") {
		return "", fmt.Errorf("input %q contains a forbidden control character", name)
	}
	return v, nil
}

func output(name, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("multiline output rejected")
	}
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintf(f, "%s=%s\n", name, value)
		return err
	}
	fmt.Printf("%s=%s\n", name, value)
	return nil
}

func run(args ...string) error {
	cmd := exec.Command("/usr/bin/"+tool, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func capture(args ...string) (string, error) {
	cmd := exec.Command("/usr/bin/"+tool, args...)
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

func jsonMap(name string) ([]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(input(name, "{}")), &values); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object of strings: %w", name, err)
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var result []string
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, "=\n\r\x00") {
			return nil, fmt.Errorf("invalid %s key", name)
		}
		result = append(result, k+"="+values[k])
	}
	return result, nil
}

func destination(name, fallback string) (string, error) {
	p, err := filepath.Abs(input(name, fallback))
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return "", err
	}
	return p, nil
}

func archiveHash(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if err = output("archive", path); err != nil {
		return err
	}
	return output("archive-sha256", hex.EncodeToString(h.Sum(nil)))
}

func buildArgs() ([]string, error) {
	image, err := required("image")
	if err != nil {
		return nil, err
	}
	version, err := required("version")
	if err != nil {
		return nil, err
	}
	platform := input("platform", "linux/amd64")
	if platform != "linux/amd64" && platform != "linux/arm64" {
		return nil, errors.New("platform must be linux/amd64 or linux/arm64")
	}
	args := []string{"--storage-driver=vfs", "bud", "--isolation=chroot", "--network=host", "--format=oci", "--layers=false", "--platform=" + platform, "--file", input("containerfile", "Containerfile"), "--tag", image + ":" + version}
	for _, item := range []struct{ input, flag string }{{"labels", "--label"}, {"build-args", "--build-arg"}} {
		values, err := jsonMap(item.input)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			args = append(args, item.flag, v)
		}
	}
	var secrets []string
	if err := json.Unmarshal([]byte(input("secrets", "[]")), &secrets); err != nil {
		return nil, fmt.Errorf("secrets must be a JSON array: %w", err)
	}
	for _, secret := range secrets {
		args = append(args, "--secret", secret)
	}
	if target := input("target", ""); target != "" {
		args = append(args, "--target", target)
	}
	return append(args, "--", input("context", ".")), nil
}

func build() error {
	args, err := buildArgs()
	if err != nil {
		return err
	}
	if err = run(args...); err != nil {
		return fmt.Errorf("Buildah failed; see docs/runners.md for nested build requirements: %w", err)
	}
	path, err := destination("output", "image.oci.tar")
	if err != nil {
		return err
	}
	ref := input("image", "") + ":" + input("version", "")
	if err = run("--storage-driver=vfs", "push", ref, "oci-archive:"+path); err != nil {
		return err
	}
	return archiveHash(path)
}

func auth() (func(), error) {
	password := input("registry-password", "")
	if password == "" {
		return func() {}, nil
	}
	registry, err := required("registry")
	if err != nil {
		return nil, err
	}
	username, err := required("registry-username")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "registry-auth-")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	data, _ := json.Marshal(map[string]any{"auths": map[string]any{registry: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(username + ":" + password))}}})
	path := filepath.Join(dir, "config.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		cleanup()
		return nil, err
	}
	_ = os.Setenv("REGISTRY_AUTH_FILE", path)
	_ = os.Setenv("DOCKER_CONFIG", dir)
	return cleanup, nil
}

func copyImage(op string) error {
	source, err := required("source")
	if err != nil {
		return err
	}
	dest, err := required("destination")
	if err != nil {
		return err
	}
	if op == "export-image" && !strings.HasPrefix(dest, "oci-archive:") {
		return errors.New("export destination must start with oci-archive:")
	}
	if op == "import-image" && (!strings.HasPrefix(source, "oci-archive:") || !strings.HasPrefix(dest, "oci:")) {
		return errors.New("import requires an oci-archive: source and oci: layout destination")
	}
	if op == "publish-image" {
		if !strings.HasPrefix(source, "oci-archive:") && !strings.HasPrefix(source, "oci:") {
			return errors.New("publication requires a tested OCI archive or layout")
		}
		if !strings.HasPrefix(dest, "docker://") {
			return errors.New("publication destination must start with docker://")
		}
	}
	for _, ref := range []string{source, dest} {
		if !strings.HasPrefix(ref, "oci:") && !strings.HasPrefix(ref, "oci-archive:") && !strings.HasPrefix(ref, "docker://") {
			return errors.New("supported transports: oci:, oci-archive:, docker://")
		}
	}
	args := []string{"copy", "--all", "--preserve-digests", source, dest}
	if err = run(args...); err != nil {
		return err
	}
	return output("destination", dest)
}

func repositoryName(reference string) string {
	repository := strings.Split(strings.TrimPrefix(reference, "docker://"), "@")[0]
	if colon := strings.LastIndex(repository, ":"); colon > strings.LastIndex(repository, "/") {
		repository = repository[:colon]
	}
	return repository
}

func digest() error {
	ref, err := required("image")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref, "docker://") {
		ref = "docker://" + ref
	}
	if parts := strings.Split(ref, "@"); len(parts) == 2 {
		ref = "docker://" + repositoryName(ref) + "@" + parts[1]
	}
	// Hash the raw index, not the platform-selected manifest returned by inspect.
	cmd := exec.Command("/usr/bin/"+tool, "inspect", "--raw", ref)
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	value := "sha256:" + hex.EncodeToString(h[:])
	if expected := input("expected-digest", ""); expected != "" && expected != value {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expected, value)
	}
	if err = output("digest", value); err != nil {
		return err
	}
	return output("reference", repositoryName(ref)+"@"+value)
}

// OIDCToken requests the same Sigstore audience used by the signing action.
func OIDCToken() (string, error) { return oidcToken() }

func oidcToken() (string, error) {
	endpoint := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	bearer := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if endpoint == "" || bearer == "" {
		return "", errors.New("GitHub OIDC unavailable: enable id-token: write and pass ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN to this Docker action")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", errors.New("OIDC endpoint must use HTTPS")
	}
	q := u.Query()
	q.Set("audience", "sigstore")
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("OIDC redirect refused") }}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC endpoint returned HTTP %d", res.StatusCode)
	}
	var payload struct {
		Value string `json:"value"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Value == "" {
		return "", errors.New("OIDC response contains no token")
	}
	return payload.Value, nil
}

func signature(op string) error {
	if op == "new-image-signature" && input("enabled", "true") == "false" {
		return output("signed", "false")
	}
	ref, err := required("image")
	if err != nil {
		return err
	}
	parts := strings.Split(ref, "@")
	if len(parts) != 2 || !digestPattern.MatchString(parts[1]) {
		return errors.New("signature operations require image@sha256:<digest>")
	}
	transparency := input("transparency-log", "true")
	if transparency != "true" && transparency != "false" {
		return errors.New("transparency-log must be true or false")
	}
	if transparency == "false" && input("key", "") == "" {
		return errors.New("disabling the transparency log requires an explicit key; keyless always uses Sigstore")
	}
	if op == "new-image-signature" {
		args := []string{"sign", "--yes"}
		if transparency == "false" {
			directory, err := os.MkdirTemp("", "cosign-config-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(directory)
			config := filepath.Join(directory, "signing.json")
			if err = run("signing-config", "create", "--out", config); err != nil {
				return err
			}
			args = append(args, "--signing-config", config)
		}
		if key := input("key", ""); key != "" {
			args = append(args, "--key", key)
		} else {
			token, err := oidcToken()
			if err != nil {
				return err
			}
			args = append(args, "--identity-token", token)
		}
		if err = run(append(args, ref)...); err != nil {
			return err
		}
		return output("signed", "true")
	}
	args := []string{"verify", "--output=json"}
	if transparency == "false" {
		args = append(args, "--insecure-ignore-tlog")
	}
	if key := input("key", ""); key != "" {
		args = append(args, "--key", key)
	} else {
		identity, err := required("identity")
		if err != nil {
			return err
		}
		issuer, err := required("issuer")
		if err != nil {
			return err
		}
		args = append(args, "--certificate-identity", identity, "--certificate-oidc-issuer", issuer)
	}
	report, err := capture(append(args, ref)...)
	if err != nil {
		return err
	}
	path, err := destination("output", "cosign.json")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, []byte(report+"\n"), 0644); err != nil {
		return err
	}
	return output("report", path)
}

func sbom() error {
	source, err := required("source")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(source, "oci-archive:") && !strings.HasPrefix(source, "oci-dir:") && !strings.HasPrefix(source, "dir:") && !strings.HasPrefix(source, "registry:") {
		return errors.New("source must explicitly use oci-archive:, oci-dir:, dir: or registry:")
	}
	path, err := destination("output", "sbom.spdx.json")
	if err != nil {
		return err
	}
	format := input("format", "spdx-json")
	if format != "spdx-json" && format != "cyclonedx-json" {
		return errors.New("format must be spdx-json or cyclonedx-json")
	}
	if err = run("scan", source, "--output", format+"="+path); err != nil {
		return err
	}
	return output("sbom", path)
}

func scan() error {
	source, err := required("sbom")
	if err != nil {
		return err
	}
	threshold := input("fail-on", "high")
	if !map[string]bool{"negligible": true, "low": true, "medium": true, "high": true, "critical": true}[threshold] {
		return errors.New("invalid fail-on severity")
	}
	update := input("update-database", "true")
	if update != "true" && update != "false" {
		return errors.New("update-database must be true or false")
	}
	_ = os.Setenv("GRYPE_DB_AUTO_UPDATE", update)
	path, err := destination("output", "grype.json")
	if err != nil {
		return err
	}
	err = run("sbom:"+source, "--fail-on", threshold, "--output", "json", "--file", path)
	if outErr := output("report", path); outErr != nil {
		return outErr
	}
	return err // Preserve Grype's threshold failure, keeping the report.
}

func merge() error {
	var sources []string
	if err := json.Unmarshal([]byte(input("sources", "[]")), &sources); err != nil || len(sources) != 2 {
		return errors.New("sources must be a JSON array of two tested archive paths (amd64, arm64)")
	}
	dest, err := required("destination")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(dest, "docker://") || strings.ContainsAny(dest[strings.LastIndex(dest, "/")+1:], ":@") {
		return errors.New("index destination must be docker://registry/repository without a tag or digest")
	}
	layout, err := os.MkdirTemp("", "oci-index-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(layout)
	data, err := assembleIndex(sources, layout)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if err = run("copy", "--all", "--preserve-digests", "oci:"+layout+":index", dest+"@"+digest); err != nil {
		return err
	}
	path, err := destination("output", "image-index.json")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	if err = output("digest", digest); err != nil {
		return err
	}
	return output("image", strings.TrimPrefix(dest, "docker://")+"@"+digest)
}

func execute(op string) error {
	allowed := map[string]string{"build-image": "buildah", "merge-image-index": "skopeo", "export-image": "skopeo", "import-image": "skopeo", "publish-image": "skopeo", "get-image-digest": "skopeo", "new-image-signature": "cosign", "test-image-signature": "cosign", "new-sbom": "syft", "test-vulnerabilities": "grype"}
	if allowed[op] != tool {
		return fmt.Errorf("operation %q is not supported by image %s", op, tool)
	}
	cleanup, err := auth()
	if err != nil {
		return err
	}
	defer cleanup()
	switch op {
	case "build-image":
		return build()
	case "merge-image-index":
		return merge()
	case "export-image", "import-image", "publish-image":
		return copyImage(op)
	case "get-image-digest":
		return digest()
	case "new-image-signature", "test-image-signature":
		return signature(op)
	case "new-sbom":
		return sbom()
	case "test-vulnerabilities":
		return scan()
	}
	return errors.New("unknown operation")
}

func parseArguments(args, names []string) error {
	flags := flag.NewFlagSet("action", flag.ContinueOnError)
	values := map[string]*string{}
	for _, name := range names {
		values[name] = flags.String(name, "", "override INPUT_"+strings.ToUpper(name))
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments; this image implements one action")
	}
	arguments = map[string]string{}
	flags.Visit(func(f *flag.Flag) { arguments[f.Name] = *values[f.Name] })
	return nil
}

// Main is called by each action's own main.go; the operation cannot be overridden.
func Main(operation, binary string, names []string) {
	tool = binary
	version := len(os.Args) == 2 && os.Args[1] == "--tool-version"
	if !version {
		if err := parseArguments(os.Args[1:], names); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	// GitHub bind mounts are owned by the runner UID, not a fixed image UID.
	// Start as container root and drop to that UID for non-builder actions.
	if tool != "buildah" && tool != "skopeo" && os.Getuid() == 0 {
		path := os.Getenv("GITHUB_OUTPUT")
		if path == "" {
			path = "/github/workspace"
		}
		if info, err := os.Stat(path); err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			if stat.Uid != 0 {
				if err = syscall.Setgroups([]int{}); err == nil {
					err = syscall.Setgid(int(stat.Gid))
				}
				if err == nil {
					err = syscall.Setuid(int(stat.Uid))
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, "cannot adopt runner UID:", err)
					os.Exit(1)
				}
			}
		}
	}
	var err error
	if version {
		if tool == "buildah" || tool == "skopeo" {
			err = run("--version")
		} else {
			err = run("version")
		}
	} else {
		err = execute(operation)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
