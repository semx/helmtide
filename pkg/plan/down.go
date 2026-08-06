package plan

import (
	"context"

	"github.com/semx/helmtide/pkg/parallel"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/dependency"
	log "github.com/sirupsen/logrus"
)

// Down destroys all releases that exist in a plan.
func (p *Plan) Down(ctx context.Context) (err error) {
	dependenciesGraph, err := p.Graph().Reverse()
	if err != nil {
		return err
	}

	// Run hooks
	err = p.body.Lifecycle.RunPreDown(ctx)
	if err != nil {
		return err
	}

	defer func() {
		lifecycleErr := p.body.Lifecycle.RunPostDown(ctx)
		if lifecycleErr != nil {
			log.Errorf("got an error from postdown hooks: %v", lifecycleErr)
			if err == nil {
				err = lifecycleErr
			}
		}
	}()

	nodesChan := dependenciesGraph.Run()

	wg := parallel.NewWaitGroup()

	// Count only nodes that are actually emitted to the channel. Nodes whose
	// dependency failed are pruned inside the graph (IsReady marks them failed
	// and runChan drops them) and are never sent here, so they must not be
	// counted — otherwise the WaitGroup could never reach zero and Wait would
	// hang forever. Adding one per emitted node keeps the accounting exact:
	// every counted node is guaranteed to Done() via the goroutine below.
	for node := range nodesChan {
		wg.Add(1)
		go func(ctx context.Context, wg *parallel.WaitGroup, node *dependency.Node[release.Config]) {
			defer wg.Done()
			rel := node.Data
			_, err := rel.Uninstall(ctx)
			if err != nil {
				log.Errorf("failed to uninstall %s: %v", rel.Uniq(), err)
				wg.ErrChan() <- err
				node.SetFailed()
			} else {
				log.Infof("%s uninstalled", rel.Uniq())
				node.SetSucceeded()
			}
		}(ctx, wg, node)
	}

	err = wg.WaitWithContext(ctx)

	return err
}
