package plan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/pkg/parallel"
	"github.com/semx/helmtide/pkg/release"
	log "github.com/sirupsen/logrus"
)

// isRoot reports whether p is a filesystem root (for a root filepath.Dir(p) == p).
func isRoot(p string) bool {
	return p == filepath.Dir(p)
}

// resolvePath returns an absolute, symlink-free form of dir. The final target
// may not exist yet, so we resolve symlinks in the deepest existing ancestor
// and re-append the not-yet-existing tail. This defeats lexical bypasses where
// a symlinked ancestor (e.g. rootlink -> /) hides the real target.
func resolvePath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	remainder := ""
	cur := abs

	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			if remainder == "" {
				return resolved, nil
			}

			return filepath.Join(resolved, remainder), nil
		}

		// A missing component just means we keep walking up; anything else
		// (permission, not-a-directory, ...) is a real error.
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(cur)
		if parent == cur { // reached root without an existing ancestor
			if remainder == "" {
				return cur, nil
			}

			return filepath.Join(cur, remainder), nil
		}

		remainder = filepath.Join(filepath.Base(cur), remainder)
		cur = parent
	}
}

// unsafePlanDirReason resolves dir (following symlinks) and returns a non-empty
// reason when the resolved target must never be recursively deleted. It rejects
// only genuinely dangerous targets — the filesystem root, a top-level directory
// (a direct child of root, e.g. /etc reached through a rootlink symlink), the
// current working directory, and the shared temp root — while leaving ordinary
// absolute and relative plan dirs (including ../sibling) working. It also
// rejects a symlinked final component so a symlink tail is never followed or
// blindly re-appended into the returned path. The comparison is done on
// absolute, symlink-resolved paths on both sides, so a relative TMPDIR cannot
// slip past either.
//
// KNOWN LIMITATION (TOCTOU): this is path-based validation. The returned path is
// resolved at check time, but nothing prevents an attacker who controls a
// component from swapping a symlink into an ancestor AFTER this returns and
// BEFORE Export performs the rename/RemoveAll, redirecting the operation.
// Closing that race fully requires fd-based, symlink-refusing traversal
// (openat/O_NOFOLLOW), which is out of scope for this fix. We deliberately stop
// at path-based checks here; callers must treat the plan dir as trusted.
func unsafePlanDirReason(dir, wd string) (resolved, reason string) {
	if dir == "" {
		return "", "path is empty"
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Sprintf("cannot resolve %q: %v", dir, err)
	}
	abs = filepath.Clean(abs)

	// Reject a symlinked final component. Lstat does not follow the last
	// element, so this catches both live and dangling symlink tails; we must
	// never rename/RemoveAll through such a redirect nor re-append it unresolved.
	if info, lerr := os.Lstat(abs); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Sprintf("refusing to use symlinked plan directory %q", abs)
	}

	resolved, err = resolvePath(abs)
	if err != nil {
		return "", fmt.Sprintf("cannot resolve %q: %v", dir, err)
	}

	if isRoot(resolved) {
		return "", fmt.Sprintf("refusing to use filesystem root %q", resolved)
	}

	if isRoot(filepath.Dir(resolved)) {
		return "", fmt.Sprintf("refusing to use top-level directory %q (resolved from %q)", resolved, dir)
	}

	if wd != "" {
		if rwd, err := resolvePath(wd); err == nil && resolved == rwd {
			return "", fmt.Sprintf("refusing to use the current working directory %q", resolved)
		}
	}

	if rtmp, err := resolvePath(os.TempDir()); err == nil && resolved == rtmp {
		return "", fmt.Sprintf("refusing to use the shared temp root %q", resolved)
	}

	return resolved, ""
}

// validatePlanDir resolves dir (following symlinks) to an absolute path and
// rejects locations that must never be recursively deleted on export. wd is the
// current working directory (passed in so it can be exercised deterministically
// in tests).
func validatePlanDir(dir, wd string) (string, error) {
	resolved, reason := unsafePlanDirReason(dir, wd)
	if reason != "" {
		return "", fmt.Errorf("%w: %s", ErrUnsafePlanDir, reason)
	}

	return resolved, nil
}

// isSafeToRemove reports whether dir may be handed to os.RemoveAll. It applies
// the same symlink-resolved, absolute checks as validatePlanDir so that neither
// the private tmp dir nor a partial-output path is ever removed when its
// resolved form is dangerous (empty, root, top-level, cwd or temp root).
func isSafeToRemove(dir string) bool {
	if dir == "" {
		return false
	}

	wd, _ := os.Getwd()
	_, reason := unsafePlanDirReason(dir, wd)

	return reason == ""
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

// restorePlan undoes a failed export. It removes the half-written new plan at
// dest and, when a previous plan was stashed at backup, moves it back. It never
// swallows errors: if the old plan cannot be put back it is left intact at
// backup and a loud error naming that path is returned, so the operator can
// always recover manually and is never left with neither plan.
func restorePlan(dest, backup string, cause error) error {
	if backup == "" {
		// There was no previous plan to protect; best-effort clean the partial
		// output but still surface any failure.
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			return errors.Join(cause, fmt.Errorf("failed to remove incomplete plan at %s: %w", dest, rmErr))
		}

		return cause
	}

	if rmErr := os.RemoveAll(dest); rmErr != nil {
		return errors.Join(cause, fmt.Errorf(
			"PREVIOUS PLAN PRESERVED at %s: could not remove incomplete plan %s to restore it: %w; restore manually with: mv %q %q",
			backup, dest, rmErr, backup, dest,
		))
	}

	if rErr := os.Rename(backup, dest); rErr != nil {
		return errors.Join(cause, fmt.Errorf(
			"PREVIOUS PLAN PRESERVED at %s: could not move it back to %s: %w; restore manually with: mv %q %q",
			backup, dest, rErr, backup, dest,
		))
	}

	return cause
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

	// Pin the whole export to the single validated+resolved path: writes,
	// stash, and every RemoveAll below all go through dest, so validation and
	// use can never diverge onto different path strings. (See the TOCTOU note
	// on unsafePlanDirReason for the residual race this does not close.)
	p.dir = dest
	p.fullPath = filepath.Join(dest, File)

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
		return restorePlan(dest, backup, cause)
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
