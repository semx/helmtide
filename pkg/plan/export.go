package plan

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/pkg/parallel"
	"github.com/semx/helmtide/pkg/release"
	log "github.com/sirupsen/logrus"
)

// validatePlanDir resolves dir to an absolute path and rejects locations that
// must never be recursively deleted on export. wd is the current working
// directory (passed in so it can be exercised deterministically in tests).
func validatePlanDir(dir, wd string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("%w: path is empty", ErrUnsafePlanDir)
	}

	if clean := filepath.Clean(dir); clean == "." || clean == ".." {
		return "", fmt.Errorf("%w: refusing to use %q", ErrUnsafePlanDir, clean)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("%w: cannot resolve %q: %w", ErrUnsafePlanDir, dir, err)
	}
	abs = filepath.Clean(abs)

	// Filesystem root: for the root path filepath.Dir(root) == root.
	if abs == filepath.Dir(abs) {
		return "", fmt.Errorf("%w: refusing to use filesystem root %q", ErrUnsafePlanDir, abs)
	}

	if wd != "" && abs == filepath.Clean(wd) {
		return "", fmt.Errorf("%w: refusing to use the current working directory %q", ErrUnsafePlanDir, abs)
	}

	if abs == filepath.Clean(os.TempDir()) {
		return "", fmt.Errorf("%w: refusing to use the shared temp root %q", ErrUnsafePlanDir, abs)
	}

	return abs, nil
}

// isSafeToRemove reports whether dir may be handed to os.RemoveAll. It guards
// against wiping an empty path, the filesystem root or the shared temp root
// (e.g. if tmpDir allocation ever leaves a dangerous value).
func isSafeToRemove(dir string) bool {
	if dir == "" {
		return false
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)

	if abs == filepath.Dir(abs) { // filesystem root
		return false
	}

	if abs == filepath.Clean(os.TempDir()) {
		return false
	}

	return true
}

// cleanTmpDir removes the plan's private temporary directory, but never an
// empty/root/temp-root path.
func (p *Plan) cleanTmpDir() {
	if !isSafeToRemove(p.tmpDir) {
		return
	}

	if err := os.RemoveAll(p.tmpDir); err != nil {
		p.Logger().WithError(err).Error("failed to remove temporary directory")
	}
}

// stashDir atomically moves an existing dir aside to a unique sibling path and
// returns that path. If dir does not exist it returns "" and no error.
func stashDir(dir string) (string, error) {
	if !helper.IsExists(dir) {
		return "", nil
	}

	// Reserve a unique sibling name on the same filesystem so os.Rename below
	// is atomic and the later rename-back cannot cross a mount boundary.
	placeholder, err := os.MkdirTemp(filepath.Dir(dir), filepath.Base(dir)+".bak-*")
	if err != nil {
		return "", err
	}
	if err := os.Remove(placeholder); err != nil {
		return "", err
	}

	if err := os.Rename(dir, placeholder); err != nil {
		return "", err
	}

	return placeholder, nil
}

// Export allows save plan to file.
func (p *Plan) Export(ctx context.Context, skipUnchanged bool) error {
	if p.initErr != nil {
		return p.initErr
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	dest, err := validatePlanDir(p.dir, wd)
	if err != nil {
		return err
	}

	log.Tracef("I am exporting plan to %s", dest)

	// Always try to clean our private temp dir, but never the shared temp root.
	defer p.cleanTmpDir()

	// Move any existing plan aside instead of deleting it up front. On a mid
	// export failure it is restored, so the user is never left with neither the
	// old nor the new plan; on success it is removed only after the new plan is
	// complete.
	backup, err := stashDir(dest)
	if err != nil {
		return fmt.Errorf("failed to move existing plan aside: %w", err)
	}

	fail := func(cause error) error {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			p.Logger().WithError(rmErr).Error("failed to remove incomplete plan")
		}
		if backup != "" {
			if rErr := os.Rename(backup, dest); rErr != nil {
				return fmt.Errorf("%w (also failed to restore previous plan from %s: %v)", cause, backup, rErr)
			}
		}

		return cause
	}

	if skipUnchanged {
		p.removeUnchanged()
		p.Logger().Info("removed unchanged releases from plan")
	}

	wg := parallel.NewWaitGroup()
	wg.Add(4)

	go func() {
		defer wg.Done()
		if err := p.exportCharts(); err != nil {
			wg.ErrChan() <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := p.exportManifest(); err != nil {
			wg.ErrChan() <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := p.exportValues(); err != nil {
			wg.ErrChan() <- err
		}
	}()
	go func() {
		defer wg.Done()
		if err := p.exportGraphMD(); err != nil {
			wg.ErrChan() <- err
		}
	}()

	if err := wg.Wait(); err != nil {
		return fail(err)
	}

	// Save Planfile after everything is exported.
	if err := helper.SaveInterface(ctx, p.fullPath, p.body); err != nil {
		return fail(err)
	}

	// New plan is complete: drop the previous copy.
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			p.Logger().WithError(err).Warn("failed to remove previous plan backup")
		}
	}

	return nil
}

func (p *Plan) removeUnchanged() {
	p.body.Releases = slices.DeleteFunc(p.body.Releases, func(rel release.Config) bool {
		return slices.ContainsFunc(p.unchanged, func(r release.Config) bool {
			return r.Uniq().Equal(rel.Uniq())
		})
	})
}

func (p *Plan) exportCharts() error {
	for i, rel := range p.body.Releases {
		l := p.Logger().WithField("release", rel.Uniq())

		if !rel.Chart().IsRemote() {
			l.Info("chart is local, skipping exporting it")

			continue
		}

		src := path.Join(p.tmpDir, "charts", rel.Uniq().String())
		dst := path.Join(p.dir, "charts", rel.Uniq().String())
		err := helper.MoveFile(
			src,
			dst,
		)
		if err != nil {
			return err
		}

		// Chart is places as an archive under this directory.
		// So we need to find it and use.
		entries, err := os.ReadDir(dst)
		if err != nil {
			l.WithError(err).Warn("failed to read directory with downloaded chart, skipping")

			continue
		}

		if len(entries) != 1 {
			l.WithField("entries", entries).Warn("don't know which file is downloaded chart, skipping")

			continue
		}

		chart := entries[0]
		p.body.Releases[i].SetChartName(path.Join(dst, chart.Name()))
	}

	return nil
}

func (p *Plan) exportManifest() error {
	for k, v := range p.manifests {
		m := filepath.Join(p.dir, Manifest, k.String()+".yml")

		f, err := helper.CreateFile(m)
		if err != nil {
			return err
		}

		_, err = f.WriteString(v)
		if err != nil {
			return fmt.Errorf("failed to write manifest %s: %w", f.Name(), err)
		}

		err = f.Close()
		if err != nil {
			return fmt.Errorf("failed to close manifest %s: %w", f.Name(), err)
		}
	}

	return nil
}

func (p *Plan) exportGraphMD() error {
	found := slices.ContainsFunc(p.body.Releases, func(rel release.Config) bool {
		return len(rel.DependsOn()) > 0
	})
	if !found {
		return nil
	}

	const filename = "graph.md"
	f, err := helper.CreateFile(filepath.Join(p.dir, filename))
	if err != nil {
		return err
	}

	_, err = f.WriteString(p.graphMD)
	if err != nil {
		return fmt.Errorf("failed to write graph file %s: %w", f.Name(), err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close graph file %s: %w", f.Name(), err)
	}

	return nil
}

func (p *Plan) exportValues() error {
	found := false

	for i, rel := range p.body.Releases {
		for j := range p.body.Releases[i].Values() {
			found = true
			p.body.Releases[i].Values()[j].SetUniq(p.dir, rel.Uniq())
		}
	}

	if !found {
		return nil
	}

	// It doesn't work if workdir has been mounted.
	err := helper.MoveFile(
		filepath.Join(p.tmpDir, Values),
		filepath.Join(p.dir, Values),
	)
	if err != nil {
		return fmt.Errorf("failed to copy values from %s to %s: %w", p.tmpDir, p.dir, err)
	}

	return nil
}

// IsExist returns true if planfile exists.
func (p *Plan) IsExist() bool {
	return helper.IsExists(p.fullPath)
}

// IsManifestExist returns true if planfile exists.
func (p *Plan) IsManifestExist() bool {
	return helper.IsExists(filepath.Join(p.dir, Manifest))
}
