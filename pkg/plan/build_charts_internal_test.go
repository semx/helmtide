package plan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BuildChartsTestSuite struct {
	suite.Suite
}

func TestBuildChartsTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(BuildChartsTestSuite))
}

func (ts *BuildChartsTestSuite) TestBuildReleaseChart() {
	p := New(".")

	rel := NewMockReleaseConfig(ts.T())
	rel.On("DownloadChart").Return(nil)

	ts.Require().NoError(p.buildReleaseChart(rel))

	rel.AssertExpectations(ts.T())
}

func (ts *BuildChartsTestSuite) TestBuildReleaseChartError() {
	p := New(".")

	rel := NewMockReleaseConfig(ts.T())
	errExpected := errors.New(ts.T().Name())
	rel.On("DownloadChart").Return(errExpected)

	ts.Require().ErrorIs(p.buildReleaseChart(rel), errExpected)

	rel.AssertExpectations(ts.T())
}
