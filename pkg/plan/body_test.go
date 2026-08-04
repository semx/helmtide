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
		want    string
		present []string
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

			t.Chdir(dir)

			require.Equal(t, tt.want, plan.DefaultBody())
		})
	}
}

// The template of the main config follows the same rule.
func TestDefaultTpl(t *testing.T) {
	for _, tt := range []struct {
		name    string
		want    string
		present []string
	}{
		{"nothing present", plan.Tpl, nil},
		{"only ours", plan.Tpl, []string{plan.Tpl}},
		{"only the inherited name", plan.LegacyTpl, []string{plan.LegacyTpl}},
		{"both, ours wins", plan.Tpl, []string{plan.Tpl, plan.LegacyTpl}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.present {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("project: test\n"), 0o600))
			}

			t.Chdir(dir)

			require.Equal(t, tt.want, plan.DefaultTpl())
		})
	}
}
