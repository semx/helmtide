package release

import (
	"bytes"
	"os/exec"
	"path/filepath"

	"helm.sh/helm/v4/pkg/postrenderer"
)

// execPostRenderer pipes rendered manifests through an external command.
//
// helm v3 shipped this as postrender.NewExec. helm v4 dropped it: a post-renderer there is a helm
// plugin of type postrenderer/v1, named rather than executed. helmtide's `post_renderer` has always
// been a command line, so the runner lives here now to keep helmwave.yml working unchanged.
type execPostRenderer struct {
	binary string
	args   []string
}

// newExecPostRenderer resolves binary and returns a post-renderer that runs it.
// A binary without a path separator is looked up in $PATH, anything else is resolved to an
// absolute path.
func newExecPostRenderer(binary string, args ...string) (postrenderer.PostRenderer, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, NewPostRendererNotFoundError(binary, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, NewPostRendererNotFoundError(binary, err)
	}

	return &execPostRenderer{binary: abs, args: args}, nil
}

// Run implements postrenderer.PostRenderer.
func (p *execPostRenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command(p.binary, p.args...)
	cmd.Stdin = renderedManifests
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, NewPostRendererError(p.binary, stderr.String(), err)
	}

	return &stdout, nil
}
