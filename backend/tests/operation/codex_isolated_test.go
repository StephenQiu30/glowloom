package operation_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/codex"
)

func TestIsolatedCodexLauncherNeverInheritsHostSecrets(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	require.NoError(t, os.Chmod(home, 0700))
	require.NoError(t, os.Chmod(root, 0700))
	binary := filepath.Join(t.TempDir(), "codex-fixture")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = '--version' ]; then echo 'codex-cli 0.159.2'; exit 0; fi\n/usr/bin/env\n"), 0700))
	t.Setenv("OPENAI_API_KEY", "fixture-secret-not-for-child")
	t.Setenv("CODEX_HOME", "/fixture/desktop-identity")
	launcher, err := codex.NewIsolatedProcessLauncher(codex.IsolatedProcessConfig{Binary: binary, Home: home, WorkRoot: root})
	require.NoError(t, err)
	dir, err := os.MkdirTemp(root, "task-")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := launcher.Open(ctx, dir)
	require.NoError(t, err)
	output, err := io.ReadAll(conn)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.NotContains(t, string(output), "fixture-secret")
	require.NotContains(t, string(output), "desktop-identity")
	resolvedHome, err := filepath.EvalSymlinks(home)
	require.NoError(t, err)
	require.Contains(t, string(output), "CODEX_HOME="+resolvedHome)
	require.Contains(t, string(output), "HOME="+resolvedHome)
	_, err = launcher.Open(ctx, t.TempDir())
	require.Error(t, err)
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\necho 'codex-cli 0.0.0'\n"), 0700))
	_, err = launcher.Open(ctx, dir)
	require.Error(t, err)
}
func TestIsolatedCodexRejectsSharedDirectories(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	require.NoError(t, os.Chmod(home, 0700))
	require.NoError(t, os.Chmod(root, 0700))
	for _, config := range []codex.IsolatedProcessConfig{
		{Binary: "codex", Home: home, WorkRoot: root}, {Binary: "/usr/bin/false", Home: home, WorkRoot: home},
	} {
		_, err := codex.NewIsolatedProcessLauncher(config)
		require.Error(t, err)
	}
	t.Setenv("CODEX_HOME", home)
	_, err := codex.NewIsolatedProcessLauncher(codex.IsolatedProcessConfig{Binary: "/usr/bin/false", Home: home, WorkRoot: root})
	require.Error(t, err, "explicit desktop CODEX_HOME must not be reused")
	t.Setenv("CODEX_HOME", "")
	require.NoError(t, os.Chmod(home, 0755))
	_, err = codex.NewIsolatedProcessLauncher(codex.IsolatedProcessConfig{Binary: "/usr/bin/false", Home: home, WorkRoot: root})
	require.Error(t, err)
}
func TestIsolatedInstalledCodexHandshake(t *testing.T) {
	binary := os.Getenv("GLOWLOOM_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("explicit installed binary opt-in; no inference")
	}
	home, root := t.TempDir(), t.TempDir()
	require.NoError(t, os.Chmod(home, 0700))
	require.NoError(t, os.Chmod(root, 0700))
	launcher, err := codex.NewIsolatedProcessLauncher(codex.IsolatedProcessConfig{Binary: binary, Home: home, WorkRoot: root})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	status, err := codex.Probe(ctx, launcher, root)
	require.NoError(t, err)
	require.False(t, status.Authenticated, "fresh service home must not inherit desktop login")
	t.Logf("isolated pinned CLI handshake passed; authenticated=%t; models=%d; no inference", status.Authenticated, len(status.Models))
}
