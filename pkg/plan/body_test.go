package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/plan"
	"github.com/stretchr/testify/require"
)

// helmtide was forked from helmwave, so a repository that already has a
// helmwave.yml has to keep working without being renamed first.
func TestDefaultBody(t *testing.T) {
	for _, tt := range []struct {
		name    string
		present []string
		want    string
	}{
		{
			name:    "nothing present falls back to our own name",
			present: nil,
			want:    plan.Body,
		},
		{
			name:    "only helmtide.yml",
			present: []string{plan.Body},
			want:    plan.Body,
		},
		{
			name:    "only helmwave.yml is still accepted",
			present: []string{plan.LegacyBody},
			want:    plan.LegacyBody,
		},
		{
			name:    "both present, ours wins",
			present: []string{plan.Body, plan.LegacyBody},
			want:    plan.Body,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.present {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("project: test\n"), 0o600))
			}

			wd, err := os.Getwd()
			require.NoError(t, err)
			require.NoError(t, os.Chdir(dir))
			defer func() { require.NoError(t, os.Chdir(wd)) }()

			require.Equal(t, tt.want, plan.DefaultBody())
		})
	}
}

// The template of the main config follows the same rule.
func TestDefaultTpl(t *testing.T) {
	for _, tt := range []struct {
		name    string
		present []string
		want    string
	}{
		{"nothing present", nil, plan.Tpl},
		{"only ours", []string{plan.Tpl}, plan.Tpl},
		{"only the inherited name", []string{plan.LegacyTpl}, plan.LegacyTpl},
		{"both, ours wins", []string{plan.Tpl, plan.LegacyTpl}, plan.Tpl},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.present {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("project: test\n"), 0o600))
			}

			wd, err := os.Getwd()
			require.NoError(t, err)
			require.NoError(t, os.Chdir(dir))
			defer func() { require.NoError(t, os.Chdir(wd)) }()

			require.Equal(t, tt.want, plan.DefaultTpl())
		})
	}
}
