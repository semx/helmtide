//go:build integration

package release_test

import (
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/suite"
)

type ListTestSuite struct {
	suite.Suite
}

func (s *ListTestSuite) SetupTest() {
	tests.RequireCluster(s.T())
}

func (s *ListTestSuite) TestNonExistingList() {
	rel := release.NewConfig()
	rel.NameF = "blabla"
	rel.NamespaceF = "blabla"

	_, err := rel.List()

	s.Require().ErrorIs(err, release.ErrNotFound)
}

func TestListTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ListTestSuite))
}
