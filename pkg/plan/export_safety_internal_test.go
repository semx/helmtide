package plan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/template"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// TestIsRoot exercises the pure filesystem-root predicate without touching the
// real filesystem (so we never run the literal "/" case through deletion).
func TestIsRoot(t *testing.T) {
	t.Parallel()

	require.True(t, isRoot(string(filepath.Separator)))
	require.False(t, isRoot(filepath.Join(t.TempDir(), "x")))
}

// TestValidatePlanDirRejectsUnsafe proves the guard refuses the empty path, the
// current working directory and the shared temp root (resolved).
func TestValidatePlanDirRejectsUnsafe(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	cases := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"dot", "."},
		{"cwd", wd},
		{"temp-root", os.TempDir()},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			_, err := validatePlanDir(c.dir, wd)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrUnsafePlanDir)
		})
	}
}

// TestValidatePlanDirAcceptsSafe proves ordinary absolute and relative
// (../sibling) plan dirs are accepted and normalised, i.e. we do not blanket
// reject legitimate paths.
func TestValidatePlanDirAcceptsSafe(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "myplan")
	abs, err := validatePlanDir(dir, wd)
	require.NoError(t, err)
	// The result is absolute and keeps the requested leaf (it may differ from
	// dir only by having symlinks in the ancestors resolved).
	require.True(t, filepath.IsAbs(abs))
	require.Equal(t, "myplan", filepath.Base(abs))

	// ../sibling relative to the cwd must stay usable.
	_, err = validatePlanDir(filepath.Join("..", "some-sibling-plan"), wd)
	require.NoError(t, err)
}

// TestValidatePlanDirResolvesSymlinkedAncestor closes the parent-symlink
// bypass: a symlink (rootlink) pointing at a dangerous dir (here the shared
// temp root, used as a hermetic proxy instead of the real "/") must be rejected
// even though the lexical path "rootlink" looks innocent.
func TestValidatePlanDirResolvesSymlinkedAncestor(t *testing.T) {
	// Not parallel: mutates TMPDIR so os.TempDir() points at our proxy dir.
	proxyRoot := t.TempDir()
	t.Setenv("TMPDIR", proxyRoot)

	wd := t.TempDir()
	rootlink := filepath.Join(wd, "rootlink")
	require.NoError(t, os.Symlink(proxyRoot, rootlink))

	// "rootlink" resolves to proxyRoot == os.TempDir(); a purely lexical check
	// would let it through.
	_, err := validatePlanDir(rootlink, wd)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafePlanDir)
}

// TestValidatePlanDirRejectsSymlinkTail proves a symlinked final component
// (whether dangling or pointing at a real dir) is rejected rather than followed
// or re-appended unresolved.
func TestValidatePlanDirRejectsSymlinkTail(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	base := t.TempDir()

	// Dangling symlink tail (target does not exist).
	dangling := filepath.Join(base, "dangling")
	require.NoError(t, os.Symlink(filepath.Join(base, "nowhere"), dangling))
	_, err = validatePlanDir(dangling, wd)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafePlanDir)

	// Live symlink tail (points at a real dir).
	target := filepath.Join(base, "real")
	require.NoError(t, os.MkdirAll(target, 0o755))
	live := filepath.Join(base, "live")
	require.NoError(t, os.Symlink(target, live))
	_, err = validatePlanDir(live, wd)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafePlanDir)
}

// TestValidatePlanDirRelativeTempDir closes the relative-TMPDIR bypass: when
// os.TempDir() is relative, both sides must be made absolute+resolved before
// comparison.
func TestValidatePlanDirRelativeTempDir(t *testing.T) {
	// Not parallel: mutates TMPDIR to a relative value.
	t.Setenv("TMPDIR", "reltmp")

	wd, err := os.Getwd()
	require.NoError(t, err)

	_, err = validatePlanDir("reltmp", wd)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafePlanDir)
}

// TestIsSafeToRemove proves the tmpDir removal guard never wipes an
// empty/temp-root path but allows a genuine private temp subdir.
func TestIsSafeToRemove(t *testing.T) {
	t.Parallel()

	require.False(t, isSafeToRemove(""))
	require.False(t, isSafeToRemove(os.TempDir()))
	require.True(t, isSafeToRemove(filepath.Join(t.TempDir(), "x")))
}

// TestExportRejectsUnsafePlanDir proves Export bails out with ErrUnsafePlanDir
// before touching the filesystem for destructive --plandir values.
func TestExportRejectsUnsafePlanDir(t *testing.T) {
	t.Parallel()

	ctx := tests.GetContext(t)

	for _, dir := range []string{"", "."} {
		p := New(filepath.Join(t.TempDir(), "seed"))
		p.body = &planBody{}
		p.dir = dir
		p.fullPath = filepath.Join(dir, File)

		err := p.Export(ctx, false)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrUnsafePlanDir)
	}
}

// TestExportRejectsTempRootKeepsContents proves that pointing --plandir at the
// shared temp root is rejected and nothing inside it is deleted.
func TestExportRejectsTempRootKeepsContents(t *testing.T) {
	// Not parallel: mutates TMPDIR so os.TempDir() resolves to a scratch dir.
	root := t.TempDir()
	t.Setenv("TMPDIR", root)

	ctx := tests.GetContext(t)

	sentinel := filepath.Join(root, "keepme.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("important"), 0o600))

	p := New(root)
	p.body = &planBody{}
	p.dir = root // == os.TempDir()
	p.fullPath = filepath.Join(root, File)

	err := p.Export(ctx, false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsafePlanDir)
	require.FileExists(t, sentinel)
}

// TestNewNoTempDirFallback proves that when MkdirTemp fails New does NOT alias
// tmpDir to the shared temp root: the plan is marked unusable and Build/Export
// surface the error.
func TestNewNoTempDirFallback(t *testing.T) {
	// Not parallel: mutates TMPDIR to a non-existent dir so MkdirTemp fails.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("TMPDIR", missing)

	ctx := tests.GetContext(t)

	p := New(filepath.Join(t.TempDir(), "plan"))
	require.Error(t, p.initErr)
	require.Empty(t, p.tmpDir)                  // no fallback allocation
	require.NotEqual(t, os.TempDir(), p.tmpDir) // never aliased to the temp root

	require.ErrorIs(t, p.Build(ctx, BuildOptions{}), p.initErr)
	require.ErrorIs(t, p.Export(ctx, false), p.initErr)
}

// TestExportRestoresPreviousPlanOnFailure proves that when the new export fails
// partway the previously exported plan is restored intact.
func TestExportRestoresPreviousPlanOnFailure(t *testing.T) {
	t.Parallel()

	ctx := tests.GetContext(t)

	root := t.TempDir()
	dest := filepath.Join(root, "plan")
	require.NoError(t, os.MkdirAll(dest, 0o755))

	oldMarker := filepath.Join(dest, "OLDPLAN")
	require.NoError(t, os.WriteFile(oldMarker, []byte("old"), 0o600))

	localChart := filepath.Join(root, "chart.tgz")
	require.NoError(t, os.WriteFile(localChart, []byte("chart"), 0o600))

	p := New(dest)
	p.templater = template.TemplaterSprig

	// Release declares values that were never built into tmpDir, so
	// exportValues fails mid-export, after the old plan was staged aside.
	mockedRelease := NewMockReleaseConfig(t)
	mockedRelease.On("Name").Return("redis")
	mockedRelease.On("Namespace").Return("default")
	mockedRelease.On("KubeContext").Return("")
	mockedRelease.On("Uniq").Return()
	mockedRelease.On("Chart").Return(&release.Chart{Name: localChart})
	mockedRelease.On("DependsOn").Return([]*release.DependsOnReference(nil))
	mockedRelease.On("Values").Return([]release.ValuesReference{
		{Src: filepath.Join(root, "values.yaml")},
	})

	p.body = &planBody{Releases: release.Configs{mockedRelease}}

	err := p.Export(ctx, false)
	require.Error(t, err)

	require.FileExists(t, oldMarker)
	require.NoFileExists(t, filepath.Join(dest, File))
}

// TestRestorePlanFailureIsLoud proves that if the old plan cannot be moved back
// it is left intact at the backup path and a loud, non-swallowed error naming
// that path is returned, so the user is never left with neither plan.
func TestRestorePlanFailureIsLoud(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based failure injection does not work as root")
	}

	parent := t.TempDir()
	dest := filepath.Join(parent, "plan")
	backup := filepath.Join(parent, "plan.bak")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, os.MkdirAll(backup, 0o755))

	backupMarker := filepath.Join(backup, "OLDPLAN")
	require.NoError(t, os.WriteFile(backupMarker, []byte("old"), 0o600))

	// Make the parent read-only so RemoveAll(dest) cannot rmdir dest, forcing
	// the restore to fail without ever destroying the backup.
	require.NoError(t, os.Chmod(parent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	cause := errors.New("simulated mid-export failure")
	err := restorePlan(dest, backup, cause)

	require.Error(t, err)
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "PREVIOUS PLAN PRESERVED")
	require.ErrorContains(t, err, backup)

	// The previous plan must still be recoverable at the backup path.
	require.FileExists(t, backupMarker)
}
