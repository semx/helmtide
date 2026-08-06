package release_test

import (
	"slices"
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/stretchr/testify/suite"
)

type ConfigTestSuite struct {
	suite.Suite
}

func TestConfigTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ConfigTestSuite))
}

func (s *ConfigTestSuite) TestConfigUniq() {
	r := release.NewConfig()
	r.NameF = "redis"
	r.NamespaceF = "test"
	r.KubeContextF = "ctx"

	s.Require().NoError(r.Uniq().Validate())
}

func (s *ConfigTestSuite) TestConfigUniqTags() {
	r := release.NewConfig()

	r.BuildAfterUnmarshal()

	s.Require().True(slices.Contains(r.TagsF, r.Uniq().String()))
}

// TestTagDependencyExcludesSelf ensures a release depending on a tag it also
// carries does not become a dependency of itself (which would form a cycle),
// while still depending on the other releases sharing that tag.
func (s *ConfigTestSuite) TestTagDependencyExcludesSelf() {
	r1 := release.NewConfig()
	r1.NameF = "weba"
	r1.NamespaceF = "test"
	r1.TagsF = []string{"grp"}
	r1.DependsOnF = []*release.DependsOnReference{{Tag: "grp"}}

	r2 := release.NewConfig()
	r2.NameF = "webb"
	r2.NamespaceF = "test"
	r2.TagsF = []string{"grp"}

	r1.BuildAfterUnmarshal(r1, r2)

	deps := r1.DependsOn()
	s.Require().Len(deps, 1)
	s.Equal(r2.Uniq().String(), deps[0].Name)

	for _, dep := range deps {
		s.NotEqual(r1.Uniq().String(), dep.Name, "release must not depend on itself")
	}
}

func (s *ConfigTestSuite) TestConfigInvalidUniq() {
	r := release.NewConfig()
	r.NameF = "redis"
	r.NamespaceF = ""

	s.Require().Error(r.Uniq().Validate())
}

func (s *ConfigTestSuite) TestDependsOn() {
	r := release.NewConfig()

	r.NamespaceF = "testns"
	r.KubeContextF = "testctx"
	r.DependsOnF = []*release.DependsOnReference{
		{Name: "bla"},
		{Name: "blabla@testns"},
		{Name: "blablabla@testtestns"},
		{Name: "---=-=-==-@kk;'[["},
	}

	r.BuildAfterUnmarshal(r)

	expected := []*release.DependsOnReference{
		{Name: "bla@testns@testctx"},
		{Name: "blabla@testns@testctx"},
		{Name: "blablabla@testtestns@testctx"},
	}
	s.Require().ElementsMatch(r.DependsOn(), expected)
}

func (s *ConfigTestSuite) TestDryRun() {
	rel := release.NewConfig()

	s.Require().False(rel.IsDryRun())
	rel.DryRun(true)
	s.Require().True(rel.IsDryRun())
}
