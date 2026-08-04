package release_test

import (
	"context"
	"strings"
	"testing"

	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/template"
	"github.com/semx/helmtide/tests"
	log "github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type ValuesTestSuite struct {
	suite.Suite

	ctx context.Context
}

func TestValuesTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(ValuesTestSuite))
}

func (ts *ValuesTestSuite) SetupTest() {
	ts.ctx = tests.GetContext(ts.T())
}

func (ts *ValuesTestSuite) TestProhibitDst() {
	type config struct {
		Values []release.ValuesReference
	}

	src := `
values:
- src: 1
  dst: a
- src: 2
  dst: b
`
	c := &config{}

	err := yaml.Unmarshal([]byte(src), c)
	ts.Require().NoError(err)

	err = release.ProhibitDst(c.Values)
	ts.Require().Error(err)
}

func (ts *ValuesTestSuite) TestList() {
	type config struct {
		Values []release.ValuesReference
	}

	src := `
values:
- a
- b
`
	c := &config{}

	err := yaml.Unmarshal([]byte(src), c)
	ts.Require().NoError(err)

	ts.Require().Equal(&config{
		Values: []release.ValuesReference{
			{Src: "a"},
			{Src: "b"},
		},
	}, c)
}

func (ts *ValuesTestSuite) TestMap() {
	type config struct {
		Values []release.ValuesReference
	}

	src := `
values:
- src: 1
  render: false
- src: 2
  strict: true
`
	c := &config{}

	err := yaml.Unmarshal([]byte(src), c)
	ts.Require().NoError(err)

	ts.Require().Equal(&config{
		Values: []release.ValuesReference{
			{Src: "1", Strict: false},
			{Src: "2", Strict: true},
		},
	}, c)
}

func (ts *ValuesTestSuite) TestBuildNonExistingNonStrict() {
	r := release.NewConfig()
	r.ValuesF = []release.ValuesReference{
		{
			Src:    "nonexisting.values",
			Strict: false,
		},
	}

	err := r.BuildValues(ts.ctx, ".", template.TemplaterSprig)

	ts.Require().NoError(err)
	ts.Require().Empty(r.Values())
}

// A missing values file is skipped silently enough that a typo in the path is
// indistinguishable from an intentional skip: the release quietly falls back to
// the chart defaults. The skip must name the file and the release it belongs to.
func (ts *ValuesTestSuite) TestBuildNonExistingNonStrictWarns() {
	hook := logTest.NewLocal(log.StandardLogger())
	ts.T().Cleanup(hook.Reset)

	const src = "nonexisting-values-warning.yaml"

	r := release.NewConfig()
	r.NameF = "skipped-values"
	r.NamespaceF = "testns"
	r.ValuesF = []release.ValuesReference{
		{
			Src:    src,
			Strict: false,
		},
	}

	err := r.BuildValues(ts.ctx, ts.T().TempDir(), template.TemplaterSprig)

	ts.Require().NoError(err)
	ts.Require().Empty(r.Values())

	var warning string

	for _, entry := range hook.AllEntries() {
		if entry.Level == log.WarnLevel && strings.Contains(entry.Message, src) {
			warning = entry.Message

			break
		}
	}

	ts.Require().NotEmptyf(warning, "skipping %q must be reported with the file path in the message", src)
	ts.Require().Contains(warning, r.Uniq().String(), "the warning must name the release that lost the values")
}

func (ts *ValuesTestSuite) TestBuildNonExistingStrict() {
	r := release.NewConfig()
	r.ValuesF = []release.ValuesReference{
		{
			Src:    "nonexisting.values",
			Strict: true,
		},
	}

	err := r.BuildValues(ts.ctx, ".", template.TemplaterSprig)

	ts.Require().Error(err)
}

func (ts *ValuesTestSuite) TestJSONSchema() {
	schema := (&release.ValuesReference{}).JSONSchema()

	ts.Require().NotNil(schema)

	ts.NotNil(schema.Properties.GetPair("src"))
	ts.NotNil(schema.Properties.GetPair("dst"))
	ts.NotNil(schema.Properties.GetPair("delimiter_left"))
	ts.NotNil(schema.Properties.GetPair("delimiter_right"))
	ts.NotNil(schema.Properties.GetPair("strict"))
	ts.NotNil(schema.Properties.GetPair("renderer"))
}
