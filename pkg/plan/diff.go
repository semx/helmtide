package plan

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/databus23/helm-diff/v3/diff"
	"github.com/databus23/helm-diff/v3/manifest"
	structDiff "github.com/r3labs/diff/v3"
	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/pkg/parallel"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/uniqname"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	live "helm.sh/helm/v4/pkg/release/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/resource"
)

// SkippedAnnotations is a map with all annotations to be skipped by differ.
var SkippedAnnotations = map[string][]string{
	live.HookAnnotation:               {string(live.HookTest), "test-success", "test-failure"},
	helper.RootAnnoName + "skip-diff": {"true"},
}

// DiffPlan show diff between 2 plans.
// It returns true when at least one difference was found between the plans,
// including releases that were added to or removed from the plan.
func (p *Plan) DiffPlan(b *Plan, opts *diff.Options) bool {
	visited := make(map[uniqname.UniqName]bool)
	changed := false

	log.WithField("suppress", opts.SuppressedKinds).Debug("suppress kinds for diffing")

	// Concat walks releases from both plans, so a release that exists only in the
	// old plan (a removal) or only in the new plan (an addition) is diffed against
	// an empty side and reported as a change.
	for _, rel := range slices.Concat(p.body.Releases, b.body.Releases) {
		if visited[rel.Uniq()] {
			continue
		}
		visited[rel.Uniq()] = true

		oldSpecs := parseManifests(b.manifests[rel.Uniq()], rel.Namespace())
		newSpecs := parseManifests(p.manifests[rel.Uniq()], rel.Namespace())

		if diff.Manifests(oldSpecs, newSpecs, opts, log.StandardLogger().Out) {
			changed = true
		} else {
			log.Info(rel.Uniq(), " no changes")
			p.unchanged = append(p.unchanged, rel)
		}
	}

	if !changed {
		log.Info("plan has no changes")
	}

	return changed
}

// DiffLive show diff with production releases in k8s-cluster.
// It returns true when at least one difference was found against the cluster,
// including releases that are not deployed yet and would be installed. A genuine
// failure to read the cluster or to render a release is returned as an error so
// that callers can tell a real failure apart from a detected change.
func (p *Plan) DiffLive(ctx context.Context, opts *diff.Options, threeWayMerge bool) (bool, error) {
	alive, _, err := p.GetLive(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get releases from the kubernetes cluster: %w", err)
	}

	changed := false

	for _, rel := range p.body.Releases {
		active, ok := alive[rel.Uniq()]
		if !ok {
			// The release does not exist on the cluster yet, so applying the plan
			// would install it. That is a change for detailed-exitcode purposes.
			changed = true
			rel.Logger().Info("release is not deployed yet, it would be installed")

			continue
		}

		newManifest := p.manifests[rel.Uniq()]
		oldManifest := active.Manifest
		if threeWayMerge {
			oldManifest = get3WayMergeManifests(rel, active.Manifest)
		}
		// I don't use manifest.ParseRelease
		// Because Structs are different.
		oldSpecs := parseManifests(oldManifest, rel.Namespace())
		newSpecs := parseManifests(newManifest, rel.Namespace())

		manifestChange := diff.Manifests(oldSpecs, newSpecs, opts, rel.Logger().Logger.Out)

		chartChange, err := diffCharts(ctx, active.Chart, rel, rel.Logger())
		if err != nil {
			return false, err
		}

		if manifestChange || chartChange {
			changed = true
		} else {
			rel.Logger().Info("no changes")
			p.unchanged = append(p.unchanged, rel)
		}
	}

	if !changed {
		log.Info("plan has no changes")
	}

	return changed, nil
}

func get3WayMergeManifests(rel release.Config, oldManifest string) string { //nolint:gocognit
	cfg := rel.Cfg()

	err := cfg.KubeClient.IsReachable()
	if err != nil {
		rel.Logger().WithError(err).Warn("failed to connect to k8s to run 3-way merge, skipping")

		return oldManifest
	}

	oldResources, err := cfg.KubeClient.Build(strings.NewReader(oldManifest), false)
	if err != nil {
		rel.Logger().WithError(err).Warn("failed to build old resources list for 3-way merge, skipping")

		return oldManifest
	}

	updatedManifest := ""

	err = oldResources.Visit(func(r *resource.Info, err error) error {
		if err != nil {
			return err
		}

		h := resource.NewHelper(r.Client, r.Mapping)
		currentObject, err := h.Get(r.Namespace, r.Name)
		if err != nil {
			if !apiErrors.IsNotFound(err) {
				return err //nolint:wrapcheck
			}

			return nil
		}

		out, err := yaml.Marshal(currentObject)
		if err != nil {
			return err //nolint:wrapcheck
		}
		// currentObject stores everything under 'object' key.
		// We need to get everything from this field and drop some generated parts.
		var ra map[string]any
		_ = yaml.Unmarshal(out, &ra)
		obj := ra["object"].(map[string]any) //nolint:forcetypeassert
		delete(obj, "status")

		metadata := obj["metadata"].(map[string]any) //nolint:forcetypeassert
		delete(metadata, "creationTimestamp")
		delete(metadata, "generation")
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "uid")

		if a := metadata["annotations"]; a != nil {
			annotations := a.(map[string]any) //nolint:forcetypeassert
			delete(annotations, "meta.helm.sh/release-name")
			delete(annotations, "meta.helm.sh/release-namespace")
			delete(annotations, "deployment.kubernetes.io/revision")

			if len(annotations) == 0 {
				delete(metadata, "annotations")
			}
		}

		out, _ = yaml.Marshal(obj)
		updatedManifest += "\n---\n" + string(out)

		return nil
	})
	if err != nil {
		rel.Logger().WithError(err).Warn("failed to get latest objects for 3-way merge, skipping")

		return oldManifest
	}

	return updatedManifest
}

//nolint:gocritic // cannot change argument types as it is required by diff library
func diffChartsFilter(path []string, _ reflect.Type, _ reflect.StructField) bool {
	return len(path) >= 1 && path[0] == "Metadata"
}

// diffCharts reports whether the chart metadata changed. A dry-run render or a
// diff failure is returned as an error instead of being swallowed, so a real
// rendering problem is not silently reported as "no change".
func diffCharts(ctx context.Context, oldChart *chart.Chart, rel release.Config, l log.FieldLogger) (bool, error) {
	l.Info("getting charts diff")

	dryRunRelease, err := rel.SyncDryRun(ctx, false)
	if err != nil {
		return false, fmt.Errorf("failed to get dry-run release for %s: %w", rel.Uniq(), err)
	}

	newChart := dryRunRelease.Chart

	changelog, err := structDiff.Diff(oldChart, newChart, structDiff.Filter(diffChartsFilter))
	if err != nil {
		return false, fmt.Errorf("failed to diff charts for %s: %w", rel.Uniq(), err)
	}

	if len(changelog) == 0 {
		return false, nil
	}

	for i := range changelog {
		change := changelog[i]
		l.WithField("path", strings.Join(change.Path, ".")).Infof("changed %q -> %q", change.From, change.To)
	}

	return true, nil
}

func parseManifests(m, ns string) map[string]*manifest.MappingResult {
	manifests := manifest.Parse(m, ns, true)

	type annotationManifest struct {
		Metadata struct {
			Annotations map[string]string
		}
	}

	for k := range manifests {
		parsed := annotationManifest{}

		if err := yaml.Unmarshal([]byte(manifests[k].Content), &parsed); err != nil {
			log.WithError(err).WithField("content", manifests[k].Content).Debug("failed to decode manifest")

			continue
		}

		for anno := range parsed.Metadata.Annotations {
			if !slices.Contains(SkippedAnnotations[anno], parsed.Metadata.Annotations[anno]) {
				continue
			}

			log.WithFields(log.Fields{
				"resource":   manifests[k].Name,
				"annotation": anno,
			}).Debug("resource diff is skipped due to annotation")
			delete(manifests, k)
		}
	}

	return manifests
}

// GetLive returns maps of releases in a k8s-cluster.
//
// A release that is simply absent from the cluster (driver.ErrReleaseNotFound)
// is reported via notFound, not as an error: the caller treats it as a release
// that would be installed. Any other lookup failure (RBAC, network, ...) is a
// genuine error and is returned so it is not misreported as a missing release.
func (p *Plan) GetLive(
	ctx context.Context,
) (found map[uniqname.UniqName]*live.Release, notFound []uniqname.UniqName, err error) {
	wg := parallel.NewWaitGroup()
	wg.Add(len(p.body.Releases))

	found = make(map[uniqname.UniqName]*live.Release)
	mu := &sync.Mutex{}

	var getErrs []error

	for i := range p.body.Releases {
		go func(wg *parallel.WaitGroup, mu *sync.Mutex, rel release.Config) {
			defer wg.Done()

			r, err := rel.Get(0)

			mu.Lock()
			defer mu.Unlock()

			switch {
			case err == nil:
				//nolint:revive // we are under mutex here
				found[rel.Uniq()] = r
			case errors.Is(err, release.ErrNotFound):
				//nolint:revive // we are under mutex here
				notFound = append(notFound, rel.Uniq())
			default:
				log.Warnf("failed to get release %s from k8s: %v", rel.Uniq(), err)
				//nolint:revive // we are under mutex here
				getErrs = append(getErrs, err)
			}
		}(wg, mu, p.body.Releases[i])
	}

	if err := wg.WaitWithContext(ctx); err != nil {
		return nil, nil, err
	}

	if len(getErrs) > 0 {
		return nil, nil, errors.Join(getErrs...)
	}

	return found, notFound, nil
}
