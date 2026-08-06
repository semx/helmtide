package plan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/semx/helmtide/pkg/release/dependency"

	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/pkg/hooks"
	"github.com/semx/helmtide/pkg/monitor"
	"github.com/semx/helmtide/pkg/registry"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/uniqname"
	"github.com/semx/helmtide/pkg/repo"
	"github.com/semx/helmtide/pkg/version"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	// Dir is the default directory for generated files.
	Dir = ".helmtide/"

	// File is the default file name for planfile.
	File = "planfile"

	// Body is a default file name for the main config.
	Body = "helmtide.yml"

	// LegacyBody is the config file name of helmwave, which helmtide was forked
	// from. It is still accepted so an existing repository works unchanged.
	LegacyBody = "helmwave.yml"

	// Tpl is the default template for the main config.
	Tpl = "helmtide.yml.tpl"

	// LegacyTpl is the template name inherited from helmwave, still accepted.
	LegacyTpl = "helmwave.yml.tpl"

	// Manifest is the default directory under Dir for manifests.
	Manifest = "manifest/"

	// Values is default directory for values.
	Values = "values/"
)

// Plan contains full helmwave state.
type Plan struct {
	body      *planBody
	dir       string
	fullPath  string
	tmpDir    string
	graphMD   string
	templater string

	// initErr is set when New failed to allocate a private temporary
	// directory. It makes the plan unusable so the failure surfaces on
	// Build/Export instead of silently aliasing tmpDir to the shared temp root.
	initErr error

	manifests map[uniqname.UniqName]string
	unchanged release.Configs
}

// NewAndImport wrapper for New and Import in one.
func NewAndImport(ctx context.Context, src string) (p *Plan, err error) {
	p = New(src)

	err = p.Import(ctx)
	if err != nil {
		return p, err
	}

	return p, nil
}

// Logger will pretty build log.Entry.
func (p *Plan) Logger() *log.Entry {
	a := helper.SlicesMap(p.body.Releases, func(r release.Config) string {
		return r.Uniq().String()
	})

	b := helper.SlicesMap(p.body.Repositories, func(r repo.Config) string {
		return r.Name()
	})

	c := helper.SlicesMap(p.body.Registries, func(r registry.Config) string {
		return r.Host()
	})

	return log.WithFields(log.Fields{
		"releases":     a,
		"repositories": b,
		"registries":   c,
	})
}

//nolint:lll
type planBody struct {
	Project      string           `yaml:"project" json:"project" jsonschema:"title=project name,description=reserved for future,example=my-awesome-project"`
	Version      string           `yaml:"version" json:"version" jsonschema:"title=version of helmwave,description=will check current version and project version,pattern=^[0-9]+\\.[0-9]+\\.[0-9]+$,example=0.23.0,example=0.22.1"`
	Monitors     monitor.Configs  `yaml:"monitors" json:"monitors" jsonschema:"title=monitors list"`
	Repositories repo.Configs     `yaml:"repositories" json:"repositories" jsonschema:"title=repositories list,description=helm repositories"`
	Registries   registry.Configs `yaml:"registries" json:"registries" jsonschema:"title=registries list,description=helm OCI registries"`
	Releases     release.Configs  `yaml:"releases" json:"releases" jsonschema:"title=helm releases,description=what you wanna deploy"`
	Lifecycle    hooks.Lifecycle  `yaml:"lifecycle" json:"lifecycle" jsonschema:"title=lifecycle,description=helmwave lifecycle hooks"`
}

// NewBody parses plan from file.
func NewBody(_ context.Context, file string, validate bool) (*planBody, error) {
	b := &planBody{
		Version: version.Version,
	}

	src, err := os.ReadFile(file)
	if err != nil {
		return b, fmt.Errorf("failed to read plan file %s: %w", file, err)
	}

	decoder := yaml.NewDecoder(bytes.NewBuffer(src))
	err = decoder.Decode(b)
	if err != nil {
		return b, fmt.Errorf("failed to unmarshal YAML plan %s: %w", file, err)
	}

	if validate {
		err = b.Validate()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// New returns empty *Plan for provided directory.
func New(dir string) *Plan {
	plan := &Plan{
		dir:       dir,
		fullPath:  filepath.Join(dir, File),
		manifests: make(map[uniqname.UniqName]string),
	}

	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		// Do NOT fall back to os.TempDir(). tmpDir is passed to os.RemoveAll
		// on export, so aliasing it to the shared temp root would recursively
		// delete the system temp directory. Leave tmpDir empty and mark the
		// plan unusable so the error surfaces on Build/Export instead.
		log.WithError(err).Error("failed to create temporary directory")
		plan.initErr = fmt.Errorf("failed to create temporary directory: %w", err)

		return plan
	}

	plan.tmpDir = tmpDir

	return plan
}

func (p *Plan) Graph() *dependency.Graph[uniqname.UniqName, release.Config] {
	graph, err := p.body.generateDependencyGraph()
	if err != nil {
		p.Logger().Fatal(err)
	}

	return graph
}

func (p *planBody) generateDependencyGraph() (*dependency.Graph[uniqname.UniqName, release.Config], error) {
	graph := dependency.NewGraph[uniqname.UniqName, release.Config]()

	for _, rel := range p.Releases {
		err := graph.NewNode(rel.Uniq(), rel)
		if err != nil {
			return nil, err
		}

		for _, dep := range rel.DependsOn() {
			graph.AddDependency(rel.Uniq(), dep.Uniq())
		}
	}

	err := graph.Build()
	if err != nil {
		return nil, err
	}

	return graph, nil
}
