package fixtures

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RustFiles is a dependency-free application fixture; no cloud credentials or
// network package registries are needed. The real application is tested by its CI.
func RustFiles() map[string]string {
	files := map[string]string{
		"Cargo.toml": "[package]\nname = \"niri-tonic-files\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"Cargo.lock": "version = 4\n\n[[package]]\nname = \"niri-tonic-files\"\nversion = \"0.1.0\"\n",
		"src/main.rs": `use std::env;

#[link(name = "gtk-4")]
extern "C" {
    fn gtk_init();
    fn gtk_window_new() -> *mut std::ffi::c_void;
    fn gtk_window_present(window: *mut std::ffi::c_void);
    fn gtk_window_destroy(window: *mut std::ffi::c_void);
    fn gtk_widget_get_mapped(widget: *mut std::ffi::c_void) -> i32;
}
#[link(name = "glib-2.0")]
extern "C" {
    fn g_main_context_iteration(context: *mut std::ffi::c_void, block: i32) -> i32;
}

fn main() {
    match env::args().nth(1).as_deref() {
        Some("--version") => println!("niri-tonic-files {}", env!("NIRI_TONIC_RELEASE_VERSION")),
        Some("--check-config") => println!("configuration valid"),
        Some("--smoke-test") => unsafe {
            assert_eq!(env::var("GDK_BACKEND").unwrap(), "wayland");
            gtk_init();
            let window = gtk_window_new();
            gtk_window_present(window);
            for _ in 0..100 {
                g_main_context_iteration(std::ptr::null_mut(), 0);
                if gtk_widget_get_mapped(window) != 0 {
                    gtk_window_destroy(window);
                    println!("synthetic smoke passed");
                    return;
                }
                std::thread::sleep(std::time::Duration::from_millis(10));
            }
            panic!("fixture GTK window did not map");
        },
        _ => std::process::exit(2),
    }
}

#[test]
fn fixture_test() {
    assert_eq!(2 + 2, 4);
}
`,
		".cargo/config.toml": "[env]\nNIRI_TONIC_RELEASE_VERSION = { value = \"0.1.0\", force = false }\n",
		".gitignore":         "target/\nnode_modules/\n.build/\n",
		"data/io.github.tdesaules.NiriTonicFiles.desktop": "[Desktop Entry]\nType=Application\nName=Fixture\nExec=niri-tonic-files\nIcon=io.github.tdesaules.NiriTonicFiles\nCategories=Utility;\n",
		"data/io.github.tdesaules.NiriTonicFiles.svg":     "<svg xmlns=\"http://www.w3.org/2000/svg\"/>\n",
		"LICENSE":                     "Synthetic qualification fixture.\n",
		"README.md":                   "# Synthetic application\n",
		"docs/fixture.md":             "# Offline fixture\n",
		"config.example.yaml":         "fixture: true\n",
		"scripts/check-connectors.py": "import sys\nassert sys.argv[1:] == ['--protocol-only']\nprint('offline protocol fixture passed')\n",
	}
	for _, name := range []string{"filen", "google-drive"} {
		prefix := "plugins/" + name + "/"
		files[prefix+"package.json"] = `{"name":"fixture","version":"1.0.0","scripts":{"test":"node --test test/fixture.mjs"}}` + "\n"
		files[prefix+"package-lock.json"] = `{"name":"fixture","version":"1.0.0","lockfileVersion":3,"packages":{"":{"name":"fixture","version":"1.0.0"}}}` + "\n"
		files[prefix+"adapter.mjs"] = "export const fixture = true;\n"
		files[prefix+"test/fixture.mjs"] = "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\ntest('offline protocol', () => assert.equal(2 + 2, 4));\n"
	}
	return files
}

// RustRepository initializes a deterministic, real Git checkout for source-bound
// actions. Call before protecting sources; output locations are kept separate.
func RustRepository(directory string) (string, error) {
	for name, data := range RustFiles() {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			return "", err
		}
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = directory
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("fixture git: %w: %s", err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "feat: synthetic offline application"}} {
		if _, err := run(args...); err != nil {
			return "", err
		}
	}
	return run("rev-parse", "HEAD")
}
