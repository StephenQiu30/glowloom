package canvasfixture

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/adapter/yjsruntime"
)

// CommandRuntime starts the built production Canvas HTTP boundary on an ephemeral
// port. It performs real native Yjs commands; no projection is fabricated.
func CommandRuntime(t testing.TB) *yjsruntime.Client {
	t.Helper()
	root, err := filepath.Abs("../../../canvas")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "node", "--input-type=module", "-e", `import {buildApp}from'./dist/http/app.js';const s=await buildApp({token:'isolated-agent-command-runtime-token'});await s.listen(0,'127.0.0.1');console.log(s.getHttpServer().address().port);`)
	cmd.Dir = root
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := sync.OnceFunc(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	t.Cleanup(stop)
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		stop()
		t.Fatal(stderr.String())
	}
	return yjsruntime.New("http://127.0.0.1:"+scanner.Text(), "isolated-agent-command-runtime-token")
}
