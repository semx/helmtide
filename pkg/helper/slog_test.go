package helper_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/semx/helmtide/pkg/helper"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogrus points the standard logrus logger at a buffer with a JSON formatter for the
// duration of fn and returns every record it emitted, decoded. It restores the logger afterwards.
func captureLogrus(t *testing.T, level log.Level, fn func()) []map[string]any {
	t.Helper()

	std := log.StandardLogger()
	prevOut := std.Out
	prevFormatter := std.Formatter
	prevLevel := std.GetLevel()
	t.Cleanup(func() {
		std.SetOutput(prevOut)
		std.SetFormatter(prevFormatter)
		std.SetLevel(prevLevel)
	})

	buf := &bytes.Buffer{}
	std.SetOutput(buf)
	std.SetFormatter(&log.JSONFormatter{})
	std.SetLevel(level)

	fn()

	var records []map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		rec := map[string]any{}
		require.NoError(t, json.Unmarshal(line, &rec))
		records = append(records, rec)
	}

	return records
}

func TestSlogHandlerForwardsLevelsAndMessage(t *testing.T) {
	recs := captureLogrus(t, log.DebugLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.Debug("dbg")
		l.Info("inf")
		l.Warn("wrn")
		l.Error("err")
	})

	require.Len(t, recs, 4)
	assert.Equal(t, "debug", recs[0]["level"])
	assert.Equal(t, "dbg", recs[0]["msg"])
	assert.Equal(t, "info", recs[1]["level"])
	assert.Equal(t, "warning", recs[2]["level"])
	assert.Equal(t, "error", recs[3]["level"])
}

func TestSlogHandlerDebugHiddenAtInfoLevel(t *testing.T) {
	recs := captureLogrus(t, log.InfoLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.Debug("hidden")
		l.Info("shown")
	})

	require.Len(t, recs, 1)
	assert.Equal(t, "shown", recs[0]["msg"])
}

// --progress (which sets helper.Helm.Debug) promotes helm's DEBUG records to INFO so they surface without
// turning the whole logger to debug. This is the behavior the flag documents.
func TestSlogHandlerProgressPromotesDebugToInfo(t *testing.T) {
	t.Cleanup(func() { helper.Helm.Debug = false })
	helper.Helm.Debug = true

	recs := captureLogrus(t, log.InfoLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.Debug("helm progress")
		l.Info("plain info")
	})

	require.Len(t, recs, 2, "debug must be visible at info level while helper.Helm.Debug is set")
	assert.Equal(t, "info", recs[0]["level"], "promoted debug must log at info")
	assert.Equal(t, "helm progress", recs[0]["msg"])
	assert.Equal(t, "info", recs[1]["level"])
}

func TestSlogHandlerFlattensGroupsAndAttrs(t *testing.T) {
	recs := captureLogrus(t, log.DebugLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.WithGroup("outer").
			With("base", 1).
			Info("msg", slog.Group("inner", slog.String("k", "v")), slog.Int("n", 2))
	})

	require.Len(t, recs, 1)
	rec := recs[0]
	assert.Equal(t, float64(1), rec["outer.base"])
	assert.Equal(t, "v", rec["outer.inner.k"])
	assert.Equal(t, float64(2), rec["outer.n"])
}

func TestSlogHandlerEmptyKeysAndGroupsIgnored(t *testing.T) {
	recs := captureLogrus(t, log.DebugLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		// Empty group name and empty attr key must not create bogus dotted fields.
		l.WithGroup("").Info("msg", slog.String("", "dropped"), slog.Group("empty"), slog.String("keep", "yes"))
	})

	require.Len(t, recs, 1)
	rec := recs[0]
	assert.Equal(t, "yes", rec["keep"])
	_, hasEmpty := rec[""]
	assert.False(t, hasEmpty, "empty-keyed attr must be dropped")
	_, hasDot := rec["."]
	assert.False(t, hasDot, "empty group must not produce a bare-dot key")
}

func TestSlogHandlerErrorKeyBecomesLogrusError(t *testing.T) {
	sentinel := errors.New("boom")
	recs := captureLogrus(t, log.DebugLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.Error("failed", slog.Any("error", sentinel))
	})

	require.Len(t, recs, 1)
	assert.Equal(t, "boom", recs[0]["error"])
}

func TestSlogHandlerResolvesLogValuer(t *testing.T) {
	recs := captureLogrus(t, log.DebugLevel, func() {
		l := slog.New(helper.NewSlogHandler())
		l.Info("msg", slog.Any("v", logValuerStub{}))
	})

	require.Len(t, recs, 1)
	assert.Equal(t, "resolved", recs[0]["v"])
}

type logValuerStub struct{}

func (logValuerStub) LogValue() slog.Value { return slog.StringValue("resolved") }

// helm v4's kube waiters log through the global slog.Default(), not the action logger. The package
// init must point that default at logrus so those records stay in helmtide's stream.
func TestGlobalSlogDefaultRoutesThroughLogrus(t *testing.T) {
	recs := captureLogrus(t, log.InfoLevel, func() {
		// Not slog.New(helper.NewSlogHandler()) -- exercise the process-wide default the init sets.
		slog.Default().Error("pod failed", "pod", "demo")
	})

	require.Len(t, recs, 1, "package-level slog must reach logrus, not Go's stderr handler")
	assert.Equal(t, "error", recs[0]["level"])
	assert.Equal(t, "pod failed", recs[0]["msg"])
	assert.Equal(t, "demo", recs[0]["pod"])
}

func TestSlogHandlerEnabledRespectsProgress(t *testing.T) {
	std := log.StandardLogger()
	prev := std.GetLevel()
	t.Cleanup(func() {
		std.SetLevel(prev)
		helper.Helm.Debug = false
	})
	std.SetLevel(log.InfoLevel)

	h := helper.NewSlogHandler()

	helper.Helm.Debug = false
	assert.False(t, h.Enabled(context.Background(), slog.LevelDebug), "debug is off at info level")

	helper.Helm.Debug = true
	assert.True(t, h.Enabled(context.Background(), slog.LevelDebug), "progress makes debug enabled")
}
