package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise fallback paths with local fake downloads; never install system packages.
func TestUnixInstallerFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash installer")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	script, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "bad-checksum", "http-error"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			downloader := `#!/bin/bash
while [ "$#" -gt 0 ]; do
 case "$1" in -o) shift; output="$1";; https:*) url="$1";; esac
 shift
done
if [ "$AEGIS_TEST_SCENARIO" = http-error ]; then exit 22; fi
asset="aegis-linux-amd64"
if [ "$(uname -s)" = Darwin ]; then asset="aegis-macos-amd64"; fi
case "$(uname -m)" in arm64|aarch64) asset="${asset%-amd64}-arm64";; esac
if [[ "$url" = */SHA256SUMS ]]; then
 if [ "$AEGIS_TEST_SCENARIO" = bad-checksum ]; then hash=$(printf '%064d' 0)
 elif command -v sha256sum >/dev/null; then hash=$(sha256sum "${output%/*}/aegis"); hash=${hash%% *}
 else hash=$(shasum -a 256 "${output%/*}/aegis"); hash=${hash%% *}; fi
 printf '%s  %s\n' "$hash" "$asset" > "$output"
else printf '#!/bin/sh\nexit 0\n' > "$output"; fi
`
			if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(downloader), 0700); err != nil {
				t.Fatal(err)
			}
			installDir := filepath.Join(dir, "installed")
			cmd := exec.Command("bash", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "AEGIS_INSTALL_DIR="+installDir, "AEGIS_PROFILE_DIR="+filepath.Join(dir, "profiles"), "AEGIS_CONFIG_DIR="+filepath.Join(dir, "profiles", ".config"), "AEGIS_TEST_SCENARIO="+scenario)
			output, err := cmd.CombinedOutput()
			if scenario == "valid" {
				if err != nil {
					t.Fatalf("fallback failed: %v\n%s", err, output)
				}
				if _, err := os.Stat(filepath.Join(installDir, "aegis")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatalf("bad release installed: %s", output)
				}
				if _, err := os.Stat(filepath.Join(installDir, "aegis")); !os.IsNotExist(err) {
					t.Fatal("failed verification installed a binary")
				}
			}
		})
	}
}

func TestUnixPathConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell startup files")
	}
	helper, err := filepath.Abs("../../scripts/configure-path.sh")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, shell, login string
		profiles           []string
	}{
		{"zsh", "/bin/zsh", "", []string{".zshrc"}},
		{"bash", "/bin/bash", "", []string{".bashrc", ".profile"}},
		{"bash-profile", "/bin/bash", ".bash_profile", []string{".bashrc", ".bash_profile"}},
		{"bash-login", "/bin/bash", ".bash_login", []string{".bashrc", ".bash_login"}},
		{"sh", "/bin/sh", "", []string{".profile"}},
		{"fish", "/usr/bin/fish", "", []string{".config/fish/conf.d/aegis.fish"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			binaryDir := filepath.Join(root, "bin ' $(printf injected) `printf injected`")
			if err := os.MkdirAll(binaryDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(binaryDir, "aegis"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if test.login != "" {
				if err := os.WriteFile(filepath.Join(root, test.login), []byte("# Existing user configuration\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				cmd := exec.Command("bash", "-c", `source "$1"; aegis_configure_path "$2" "$3" "$4" "$5"`, "aegis-path-test", helper, root, binaryDir, test.shell, filepath.Join(root, ".config"))
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("configure failed: %v\n%s", err, output)
				}
			}
			for _, relative := range test.profiles {
				profile := filepath.Join(root, relative)
				contents, err := os.ReadFile(profile)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Count(string(contents), "# Aegis executable directory") != 1 {
					t.Fatal("repeated install duplicated PATH configuration")
				}
				if relative == test.login && !strings.Contains(string(contents), "# Existing user configuration") {
					t.Fatal("existing configuration overwritten")
				}
				evaluator := exec.Command("sh", "-c", `. "$1"; . "$1"; command -v aegis`, "aegis-path-test", profile)
				if test.name == "zsh" {
					if shell, err := exec.LookPath("zsh"); err == nil {
						evaluator = exec.Command(shell, "-c", `source "$1"; source "$1"; command -v aegis`, "aegis-path-test", profile)
					}
				}
				if test.name == "fish" {
					shell, err := exec.LookPath("fish")
					if err != nil {
						continue
					}
					evaluator = exec.Command(shell, "--no-config", "-c", `source "$argv[1]"; source "$argv[1]"; command -s aegis`, profile)
				}
				output, err := evaluator.CombinedOutput()
				if err != nil {
					t.Fatalf("startup config invalid: %v\n%s", err, output)
				}
				if strings.TrimSpace(string(output)) != filepath.Join(binaryDir, "aegis") {
					t.Fatalf("aegis not found on PATH: %q", output)
				}
			}
		})
	}
}
