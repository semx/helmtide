package release_test

import (
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v4/pkg/kube"
)

type WaitTestSuite struct {
	suite.Suite
}

func TestWaitTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(WaitTestSuite))
}

// helmwave.yml files written for helm 3 say `wait: true`, and they have to keep working.
func (s *WaitTestSuite) TestUnmarshalBool() {
	for in, expected := range map[string]release.WaitStrategy{
		"true":  release.WaitStrategyWatcher,
		"false": release.WaitStrategyHookOnly,
	} {
		var got release.WaitStrategy

		s.Require().NoError(yaml.Unmarshal([]byte(in), &got), in)
		s.Equal(expected, got, in)
	}
}

func (s *WaitTestSuite) TestUnmarshalStrategy() {
	for _, in := range []release.WaitStrategy{
		release.WaitStrategyWatcher,
		release.WaitStrategyLegacy,
		release.WaitStrategyHookOnly,
	} {
		var got release.WaitStrategy

		s.Require().NoError(yaml.Unmarshal([]byte(in), &got), in)
		s.Equal(in, got)
	}
}

func (s *WaitTestSuite) TestUnmarshalUnknown() {
	var got release.WaitStrategy

	err := yaml.Unmarshal([]byte("yes-please"), &got)

	var e *release.InvalidWaitStrategyError
	s.Require().ErrorAs(err, &e)
}

// helm refuses to build a waiter for an empty strategy, so an unset one must not stay empty.
func (s *WaitTestSuite) TestUnsetDefaultsLikeHelm() {
	var unset release.WaitStrategy

	s.Equal(kube.HookOnlyStrategy, unset.Helm())
	s.False(unset.Enabled())
}

func (s *WaitTestSuite) TestEnabled() {
	s.True(release.WaitStrategyWatcher.Enabled())
	s.True(release.WaitStrategyLegacy.Enabled())
	s.False(release.WaitStrategyHookOnly.Enabled())
}

func (s *WaitTestSuite) TestJSONSchema() {
	schema := release.WaitStrategy("").JSONSchema()

	s.Require().NotNil(schema)
	s.Require().Len(schema.OneOf, 2)
}
