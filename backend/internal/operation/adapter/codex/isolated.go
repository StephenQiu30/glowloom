package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SupportedVersion pins the protocol validated by this adapter. Updating this
// value requires regenerating schemas and running protocol/real-runtime checks.
const SupportedVersion = "0.159.2"

// IsolatedProcessConfig uses an operator-provisioned service identity. The
// adapter never imports credentials, plugins or configuration from the desktop.
type IsolatedProcessConfig struct{ Binary, Home, WorkRoot string }

// NewIsolatedProcessLauncher requires separate existing directories. This is
// configuration isolation; deployment must additionally isolate the OS account
// or container filesystem before accepting untrusted project prompts.
func NewIsolatedProcessLauncher(config IsolatedProcessConfig) (*ProcessLauncher, error) {
	if !filepath.IsAbs(config.Binary) {
		return nil, fmt.Errorf("codex executable must be absolute")
	}
	paths := []string{config.Home, config.WorkRoot}
	for i, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf("codex directories must be absolute")
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("codex directory unavailable")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("codex directories require owner-only access")
		}
		paths[i] = resolved
	}
	personal, _ := os.UserHomeDir()
	personal, _ = filepath.EvalSymlinks(personal)
	for _, desktopRoot := range []string{filepath.Join(personal, ".codex"), os.Getenv("CODEX_HOME")} {
		if desktopRoot == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(desktopRoot)
		if err == nil && within(resolved, paths[0]) {
			return nil, fmt.Errorf("codex service home must not reuse the desktop identity directory")
		}
	}
	if paths[0] == personal || within(paths[0], paths[1]) || within(paths[1], paths[0]) {
		return nil, fmt.Errorf("codex home and work directories must be separate from each other and the desktop home")
	}
	return &ProcessLauncher{binary: config.Binary, home: paths[0], workRoot: paths[1]}, nil
}
func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func (l *ProcessLauncher) isolatedCommand(ctx context.Context, dir string) (*exec.Cmd, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || !within(l.workRoot, resolved) || resolved == l.workRoot {
		return nil, fmt.Errorf("codex task directory outside work root")
	}
	runtimeHome := l.home
	runtimePath := filepath.Dir(l.binary) + ":/usr/bin:/bin"
	if l.localIdentity {
		runtimeHome, _ = os.UserHomeDir()
		runtimePath = os.Getenv("PATH")
	}
	env := []string{"HOME=" + runtimeHome, "CODEX_HOME=" + l.home, "PATH=" + runtimePath, "TMPDIR=" + resolved, "LANG=en_US.UTF-8"}
	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version := exec.CommandContext(versionCtx, l.binary, "--version")
	version.Env = env
	version.Dir = resolved
	output, err := version.Output()
	if err != nil || strings.TrimSpace(string(output)) != "codex-cli "+SupportedVersion {
		return nil, fmt.Errorf("codex runtime version unsupported")
	}
	args := []string{"app-server", "--stdio", "-c", "cli_auth_credentials_store=\"file\"", "-c", "web_search=\"disabled\"", "-c", "project_doc_max_bytes=0"}
	if l.localIdentity {
		args = append([]string{"app-server", "--stdio"}, args[4:]...)
	}
	for _, feature := range []string{"shell_tool", "unified_exec", "shell_snapshot", "apps", "plugins", "browser_use", "computer_use", "multi_agent", "multi_agent_v2", "skill_search"} {
		args = append(args, "-c", "features."+feature+"=false")
	}
	args = append(args, "-c", "features.skip_host_skill_discovery=true")
	cmd := exec.CommandContext(ctx, l.binary, args...)
	cmd.Env = env
	cmd.Dir = resolved
	return cmd, nil
}
