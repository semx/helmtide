// Command postrender is a deterministic post-renderer for the integration tests.
//
// helmtide's post_renderer is an external command: helm hands it the rendered
// manifests on stdin and reads the transformed manifests back on stdout. This
// one stamps every resource it is given with two annotations:
//
//	helmtide.io/postrendered:    "true"
//	helmtide.io/postrender-batch: "<number of docs in this invocation>"
//
// The batch size is what makes the post_render_strategy observable end to end:
//
//   - nohooks:  hooks never reach the post-renderer, so a hook resource carries
//     no annotation at all, while the one ordinary manifest is stamped batch=1.
//   - combined: hooks and manifests arrive together in a single invocation, so
//     both are stamped with the same batch size (2 for the hooked chart).
//   - separate: the post-renderer is run once per group, so each resource is
//     stamped batch=1 but the hook is still stamped (unlike nohooks).
//
// It intentionally has no third-party dependencies beyond gopkg.in/yaml.v3,
// which the module already vendors, so the tests can `go build` it anywhere.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	annotationMarker = "helmtide.io/postrendered"
	annotationBatch  = "helmtide.io/postrender-batch"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "postrender:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	// First pass: collect every non-empty document so the batch size is known
	// before anything is stamped.
	dec := yaml.NewDecoder(bytes.NewReader(raw))

	var docs []map[string]any

	for {
		doc := map[string]any{}

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("decode: %w", err)
		}

		if len(doc) == 0 {
			continue
		}

		docs = append(docs, doc)
	}

	batch := fmt.Sprintf("%d", len(docs))

	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)

	for _, doc := range docs {
		stamp(doc, batch)

		if err := enc.Encode(doc); err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}

	return enc.Close()
}

// stamp adds the marker annotations under metadata.annotations, creating the
// maps as needed.
func stamp(doc map[string]any, batch string) {
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		meta = map[string]any{}
		doc["metadata"] = meta
	}

	ann, ok := meta["annotations"].(map[string]any)
	if !ok {
		ann = map[string]any{}
		meta["annotations"] = ann
	}

	ann[annotationMarker] = "true"
	ann[annotationBatch] = batch
}
