package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "AEGIS_INSTALL_DIR="+installDir, "AEGIS_TEST_SCENARIO="+scenario)
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
