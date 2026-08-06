package plan

import (
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/uniqname"
	"github.com/stretchr/testify/suite"
)

type BuildReleasesTestSuite struct {
	suite.Suite
}

func TestBuildReleasesTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(BuildReleasesTestSuite))
}

func (ts *BuildReleasesTestSuite) TestCheckTagInclusion() {
	cases := []struct {
		targetTags  []string
		releaseTags []string
		matchAll    bool
		result      bool
	}{
		{
			targetTags:  []string{},
			releaseTags: []string{"bla"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{},
			releaseTags: []string{"bla"},
			matchAll:    true,
			result:      true,
		},
		{
			targetTags:  []string{"bla"},
			releaseTags: []string{},
			matchAll:    false,
			result:      false,
		},
		{
			targetTags:  []string{"bla"},
			releaseTags: []string{},
			matchAll:    true,
			result:      false,
		},
		{
			targetTags:  []string{"bla"},
			releaseTags: []string{"abc"},
			matchAll:    false,
			result:      false,
		},
		{
			targetTags:  []string{"bla"},
			releaseTags: []string{"abc"},
			matchAll:    true,
			result:      false,
		},
		{
			targetTags:  []string{"bla"},
			releaseTags: []string{"bla"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "2"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "2"},
			matchAll:    true,
			result:      false,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"2"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"2"},
			matchAll:    true,
			result:      false,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "2", "1"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "2", "1"},
			matchAll:    true,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "4", "1", "2"},
			matchAll:    false,
			result:      true,
		},
		{
			targetTags:  []string{"1", "2", "3"},
			releaseTags: []string{"3", "4", "1", "2"},
			matchAll:    true,
			result:      true,
		},
	}

	for i := range cases {
		c := cases[i]
		res := checkTagInclusion(c.targetTags, c.releaseTags, c.matchAll)
		ts.Equal(c.result, res, c)
	}
}

func (ts *BuildReleasesTestSuite) TestNoReleases() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))
	p.NewBody()

	releases, err := p.buildReleases(BuildOptions{})

	ts.Require().NoError(err)
	ts.Empty(releases)
}

func (ts *BuildReleasesTestSuite) TestNoMatchingReleases() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	mockedRelease := NewMockReleaseConfig(ts.T())
	mockedRelease.On("Tags").Return([]string{"bla"})

	p.SetReleases(mockedRelease)

	releases, err := p.buildReleases(BuildOptions{Tags: []string{"abc"}, MatchAll: true})
	ts.Require().NoError(err)
	ts.Empty(releases)

	mockedRelease.AssertExpectations(ts.T())
}

func (ts *BuildReleasesTestSuite) TestDuplicateReleases() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u, _ := uniqname.New(ts.T().Name(), "", "")

	rel1 := NewMockReleaseConfig(ts.T())
	rel1.On("Tags").Return(tags)
	rel1.On("Uniq").Return(u)
	rel1.On("DependsOn").Return([]*release.DependsOnReference{})
	rel1.On("SetDependsOn", []*release.DependsOnReference{}).Return()

	rel2 := NewMockReleaseConfig(ts.T())
	rel2.On("Tags").Return(tags)
	rel2.On("Uniq").Return(u)

	p.SetReleases(rel1, rel2)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})

	var e *release.DuplicateError
	ts.Require().ErrorAs(err, &e)
	ts.Equal(u, e.Uniq)

	ts.Empty(releases)

	rel1.AssertExpectations(ts.T())
	rel2.AssertExpectations(ts.T())
}

func (ts *BuildReleasesTestSuite) TestMissingRequiredDependency() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u, _ := uniqname.New(ts.T().Name(), "", "")

	rel := NewMockReleaseConfig(ts.T())
	rel.On("Tags").Return(tags)
	rel.On("Uniq").Return(u)
	rel.On("DependsOn").Return([]*release.DependsOnReference{{Name: "blabla", Optional: false}})

	p.SetReleases(rel)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.ErrorIs(err, release.ErrDepFailed)
	ts.Empty(releases)

	rel.AssertExpectations(ts.T())
}

func (ts *BuildReleasesTestSuite) TestMissingOptionalDependency() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u, _ := uniqname.New(ts.T().Name(), "", "")

	rel := NewMockReleaseConfig(ts.T())
	rel.On("Tags").Return(tags)
	rel.On("Uniq").Return(u)
	rel.On("DependsOn").Return([]*release.DependsOnReference{{Name: "blabla", Optional: true}})
	rel.On("SetDependsOn", []*release.DependsOnReference{}).Return()

	p.SetReleases(rel)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.Require().NoError(err)
	ts.Len(releases, 1)
	ts.Contains(releases, rel)

	rel.AssertExpectations(ts.T())
}

func (ts *BuildReleasesTestSuite) TestUnmatchedDependency() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u1, _ := uniqname.New(ts.T().Name(), "", "")
	u2, _ := uniqname.New("blabla", "", "")
	deps := []*release.DependsOnReference{{Name: u2.String()}}

	rel1 := NewMockReleaseConfig(ts.T())
	rel1.On("Tags").Return(tags)
	rel1.On("Uniq").Return(u1)
	rel1.On("DependsOn").Return(deps)
	rel1.On("SetDependsOn", deps).Return()

	rel2 := NewMockReleaseConfig(ts.T())
	rel2.On("Tags").Return([]string{})
	rel2.On("Uniq").Return(u2)
	rel2.On("DependsOn").Return([]*release.DependsOnReference{})
	rel2.On("SetDependsOn", []*release.DependsOnReference{}).Return()

	p.SetReleases(rel1, rel2)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.Require().NoError(err)
	ts.Len(releases, 2)
	ts.Contains(releases, rel1)
	ts.Contains(releases, rel2)

	rel1.AssertExpectations(ts.T())
	rel2.AssertExpectations(ts.T())
}

// TestMutualDependencyCycle proves that A -> B, B -> A does not infinitely
// recurse (stack-overflow) while building releases. Each release is added to the
// plan exactly once, and the resulting dependency graph is then reported as a
// loop by the graph cycle detector.
func (ts *BuildReleasesTestSuite) TestMutualDependencyCycle() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u1, _ := uniqname.New("mutuala", "", "")
	u2, _ := uniqname.New("mutualb", "", "")

	deps1 := []*release.DependsOnReference{{Name: u2.String()}}
	deps2 := []*release.DependsOnReference{{Name: u1.String()}}

	rel1 := NewMockReleaseConfig(ts.T())
	rel1.On("Tags").Return(tags)
	rel1.On("Uniq").Return(u1)
	rel1.On("DependsOn").Return(deps1)
	rel1.On("SetDependsOn", deps1).Return()

	rel2 := NewMockReleaseConfig(ts.T())
	rel2.On("Tags").Return(tags)
	rel2.On("Uniq").Return(u2)
	rel2.On("DependsOn").Return(deps2)
	rel2.On("SetDependsOn", deps2).Return()

	p.SetReleases(rel1, rel2)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.Require().NoError(err)
	ts.Len(releases, 2)
	ts.Contains(releases, rel1)
	ts.Contains(releases, rel2)

	// The cycle is left for the graph builder to detect.
	_, graphErr := p.body.generateDependencyGraph()
	ts.Require().Error(graphErr)
	ts.Contains(graphErr.Error(), "loop detected")

	rel1.AssertExpectations(ts.T())
	rel2.AssertExpectations(ts.T())
}

// TestSelfDependencyCycle proves that A -> A does not infinitely recurse.
func (ts *BuildReleasesTestSuite) TestSelfDependencyCycle() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u1, _ := uniqname.New("selfdep", "", "")

	deps := []*release.DependsOnReference{{Name: u1.String()}}

	rel := NewMockReleaseConfig(ts.T())
	rel.On("Tags").Return(tags)
	rel.On("Uniq").Return(u1)
	rel.On("DependsOn").Return(deps)
	rel.On("SetDependsOn", deps).Return()

	p.SetReleases(rel)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.Require().NoError(err)
	ts.Len(releases, 1)
	ts.Contains(releases, rel)

	_, graphErr := p.body.generateDependencyGraph()
	ts.Require().Error(graphErr)
	ts.Contains(graphErr.Error(), "loop detected")

	rel.AssertExpectations(ts.T())
}

// TestDiamondDependency proves the visited-set early-return does not drop a
// shared dependency edge. A -> {B, C} and both B -> D and C -> D. When C is
// expanded, D is already visited (added via B), yet C's edge to D must still be
// preserved: each release appears once, D is present, and SetDependsOn is
// invoked with the D edge on both B and C.
func (ts *BuildReleasesTestSuite) TestDiamondDependency() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	uA, _ := uniqname.New("diamonda", "", "")
	uB, _ := uniqname.New("diamondb", "", "")
	uC, _ := uniqname.New("diamondc", "", "")
	uD, _ := uniqname.New("diamondd", "", "")

	depsA := []*release.DependsOnReference{{Name: uB.String()}, {Name: uC.String()}}
	depsB := []*release.DependsOnReference{{Name: uD.String()}}
	depsC := []*release.DependsOnReference{{Name: uD.String()}}
	depsD := []*release.DependsOnReference{}

	relA := NewMockReleaseConfig(ts.T())
	relA.On("Tags").Return(tags)
	relA.On("Uniq").Return(uA)
	relA.On("DependsOn").Return(depsA)
	relA.On("SetDependsOn", depsA).Return()

	relB := NewMockReleaseConfig(ts.T())
	relB.On("Tags").Return(tags)
	relB.On("Uniq").Return(uB)
	relB.On("DependsOn").Return(depsB)
	relB.On("SetDependsOn", depsB).Return()

	relC := NewMockReleaseConfig(ts.T())
	relC.On("Tags").Return(tags)
	relC.On("Uniq").Return(uC)
	relC.On("DependsOn").Return(depsC)
	relC.On("SetDependsOn", depsC).Return()

	relD := NewMockReleaseConfig(ts.T())
	relD.On("Tags").Return(tags)
	relD.On("Uniq").Return(uD)
	relD.On("DependsOn").Return(depsD)
	relD.On("SetDependsOn", depsD).Return()

	p.SetReleases(relA, relB, relC, relD)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: true})
	ts.Require().NoError(err)

	// Each release appears exactly once, including the shared node D.
	ts.Len(releases, 4)
	ts.Contains(releases, relA)
	ts.Contains(releases, relB)
	ts.Contains(releases, relC)
	ts.Contains(releases, relD)

	// The diamond is acyclic, so the graph builds without a loop error.
	_, graphErr := p.body.generateDependencyGraph()
	ts.Require().NoError(graphErr)

	// AssertExpectations verifies SetDependsOn(relC, depsC) was called even though
	// D was already visited when C was expanded — i.e. the edge was not dropped.
	relA.AssertExpectations(ts.T())
	relB.AssertExpectations(ts.T())
	relC.AssertExpectations(ts.T())
	relD.AssertExpectations(ts.T())
}

func (ts *BuildReleasesTestSuite) TestDisabledDependencies() {
	tmpDir := ts.T().TempDir()
	p := New(filepath.Join(tmpDir, Dir))

	tags := []string{"bla"}
	u1, _ := uniqname.NewFromString(ts.T().Name())

	rel1 := NewMockReleaseConfig(ts.T())
	rel1.On("Tags").Return(tags)
	rel1.On("Uniq").Return(u1)
	rel1.On("SetDependsOn", []*release.DependsOnReference{}).Return()

	rel2 := NewMockReleaseConfig(ts.T())
	rel2.On("Tags").Return([]string{})

	p.SetReleases(rel1, rel2)

	releases, err := p.buildReleases(BuildOptions{Tags: tags, MatchAll: true, EnableDependencies: false})
	ts.Require().NoError(err)
	ts.Len(releases, 1)
	ts.Contains(releases, rel1)

	rel1.AssertExpectations(ts.T())
	rel2.AssertExpectations(ts.T())
}
