package codex

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// NewLocalProcessLauncher is only for trusted local administrator development trials.
// The CLI reads its existing login itself. No credential files are imported or copied.
func NewLocalProcessLauncher(binary, root string) (*ProcessLauncher, error) {
	if binary == "" {
		var err error
		binary, err = exec.LookPath("codex")
		if err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(binary) || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("local Codex paths must be absolute")
	}
	work, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(work)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("codex trial workspace must be owner-only")
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		personal, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		home = filepath.Join(personal, ".codex")
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("codex CLI home unavailable")
	}
	if within(home, work) || within(work, home) {
		return nil, fmt.Errorf("codex trial workspace must be separate from CLI home")
	}
	return &ProcessLauncher{binary: binary, home: home, workRoot: work, localIdentity: true}, nil
}

// TrialImage generates one image without imposing a user-price or settlement policy.
// Its caller must durably admit the request once and store the returned bytes.
func TrialImage(ctx context.Context, launcher Launcher, root, model, prompt string) (application.GeneratedImage, string, error) {
	if !validID(model) || strings.TrimSpace(prompt) == "" || len(prompt) > 32*1024 {
		return application.GeneratedImage{}, "not_submitted", application.ErrInvalidProviderCall
	}
	dir, err := os.MkdirTemp(root, "image-trial-")
	if err != nil {
		return application.GeneratedImage{}, "not_submitted", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	conn, err := launcher.Open(ctx, dir)
	if err != nil {
		return application.GeneratedImage{}, "not_submitted", err
	}
	client := newRPC(conn)
	defer client.close()
	thread, err := prepareThread(ctx, client, model, dir)
	if err != nil {
		return application.GeneratedImage{}, "not_submitted", err
	}
	turn, err := startTurn(ctx, client, thread, model, prompt)
	if err != nil {
		return application.GeneratedImage{}, "unknown", err
	}
	_, item, err := completedImage(ctx, client, thread, turn)
	if err != nil {
		return application.GeneratedImage{}, "unknown", err
	}
	// Native CLI 0.159.2 stores images in its own cache. Consume the
	// completed item's bounded bytes, never follow an external savedPath.
	if len(item.Result) > base64.StdEncoding.EncodedLen(maxImageBytes) {
		return application.GeneratedImage{}, "unknown", errProtocol
	}
	data, err := base64.StdEncoding.Strict().DecodeString(item.Result)
	if err != nil {
		return application.GeneratedImage{}, "unknown", errProtocol
	}
	output, data, err := decodeImage(data)
	if err != nil {
		return application.GeneratedImage{}, "unknown", err
	}
	return application.GeneratedImage{Bytes: data, MediaType: output.MIMEType}, "completed", nil
}
