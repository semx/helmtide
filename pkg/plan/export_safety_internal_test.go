package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/template"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// TestValidatePlanDirRejectsUnsafe proves the path guard refuses every location
// that must never be handed to os.RemoveAll on export.
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
		{"dotdot", ".."},
		{"root", string(filepath.Separator)},
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

// TestValidatePlanDirAcceptsSafe proves an ordinary user plan dir is accepted
// and normalised to an absolute path.
func TestValidatePlanDirAcceptsSafe(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "myplan")
	abs, err := validatePlanDir(dir, wd)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(dir), abs)
}

// TestIsSafeToRemove proves the tmpDir removal guard never wipes an
// empty/root/temp-root path.
func TestIsSafeToRemove(t *testing.T) {
	t.Parallel()

	require.False(t, isSafeToRemove(""))
	require.False(t, isSafeToRemove(string(filepath.Separator)))
	require.False(t, isSafeToRemove(os.TempDir()))
	require.True(t, isSafeToRemove(filepath.Join(t.TempDir(), "x")))
}

// TestExportRejectsUnsafePlanDir proves Export bails out with ErrUnsafePlanDir
// for the classic destructive --plandir values before touching the filesystem.
func TestExportRejectsUnsafePlanDir(t *testing.T) {
	t.Parallel()

	ctx := tests.GetContext(t)

	for _, dir := range []string{"", ".", string(filepath.Separator)} {
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

// TestExportRestoresPreviousPlanOnFailure proves that when the new export fails
// partway the previously exported plan is left intact (restored), so the user is
// never left with neither the old nor the new plan.
func TestExportRestoresPreviousPlanOnFailure(t *testing.T) {
	t.Parallel()

	ctx := tests.GetContext(t)

	root := t.TempDir()
	dest := filepath.Join(root, "plan")
	require.NoError(t, os.MkdirAll(dest, 0o755))

	oldMarker := filepath.Join(dest, "OLDPLAN")
	require.NoError(t, os.WriteFile(oldMarker, []byte("old"), 0o600))

	// A local chart file so exportCharts skips it without touching the network.
	localChart := filepath.Join(root, "chart.tgz")
	require.NoError(t, os.WriteFile(localChart, []byte("chart"), 0o600))

	p := New(dest)
	p.templater = template.TemplaterSprig

	// This release declares values but they were never built into tmpDir, so
	// exportValues fails while moving a non-existent values directory. That is
	// our mid-export failure, triggered only after the old plan has been staged
	// aside.
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

	// Previous plan restored intact, no half-written planfile left behind.
	require.FileExists(t, oldMarker)
	require.NoFileExists(t, filepath.Join(dest, File))
}
