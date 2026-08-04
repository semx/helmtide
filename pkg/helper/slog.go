package helper

import (
	"context"
	"log/slog"
	"strings"

	log "github.com/sirupsen/logrus"
)

// SlogHandler is a slog.Handler that forwards records to logrus.
//
// Helm v4 dropped the `func(format string, args ...any)` debug callback that v3 accepted and logs
// through log/slog instead. This handler keeps helm's output inside helmtide's single log stream,
// so `--log-level` and `--log-format` still control it.
type SlogHandler struct {
	// attrs are the attributes accumulated by WithAttrs, already qualified with their group prefix.
	attrs []slog.Attr
	// groups is the group prefix currently in effect, set by WithGroup.
	groups []string
}

// NewSlogHandler returns a slog.Handler writing to the standard logrus logger.
func NewSlogHandler() *SlogHandler {
	return &SlogHandler{}
}

// slogLevel converts a slog level to its logrus counterpart.
func slogLevel(level slog.Level) log.Level {
	switch {
	case level < slog.LevelInfo:
		return log.DebugLevel
	case level < slog.LevelWarn:
		return log.InfoLevel
	case level < slog.LevelError:
		return log.WarnLevel
	default:
		return log.ErrorLevel
	}
}

// Enabled implements slog.Handler.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return log.IsLevelEnabled(slogLevel(level))
}

// Handle implements slog.Handler.
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	fields := make(log.Fields, len(h.attrs)+r.NumAttrs())

	for _, a := range h.attrs {
		flattenAttr(fields, "", a)
	}

	prefix := strings.Join(h.groups, ".")
	r.Attrs(func(a slog.Attr) bool {
		flattenAttr(fields, prefix, a)

		return true
	})

	entry := log.WithFields(fields)
	if err, ok := fields[log.ErrorKey].(error); ok {
		entry = entry.WithError(err)
	}

	entry.Log(slogLevel(r.Level), r.Message)

	return nil
}

// WithAttrs implements slog.Handler.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	prefix := strings.Join(h.groups, ".")
	out := &SlogHandler{
		attrs:  make([]slog.Attr, 0, len(h.attrs)+len(attrs)),
		groups: h.groups,
	}
	out.attrs = append(out.attrs, h.attrs...)

	for _, a := range attrs {
		out.attrs = append(out.attrs, slog.Attr{Key: joinKey(prefix, a.Key), Value: a.Value})
	}

	return out
}

// WithGroup implements slog.Handler.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	out := &SlogHandler{
		attrs:  h.attrs,
		groups: make([]string, 0, len(h.groups)+1),
	}
	out.groups = append(out.groups, h.groups...)
	out.groups = append(out.groups, name)

	return out
}

// flattenAttr writes a single attribute into fields, expanding groups into dotted keys.
func flattenAttr(fields log.Fields, prefix string, a slog.Attr) {
	value := a.Value.Resolve()

	if value.Kind() == slog.KindGroup {
		group := value.Group()
		if len(group) == 0 {
			return
		}

		inner := prefix
		if a.Key != "" {
			inner = joinKey(prefix, a.Key)
		}

		for _, sub := range group {
			flattenAttr(fields, inner, sub)
		}

		return
	}

	if a.Key == "" {
		return
	}

	fields[joinKey(prefix, a.Key)] = value.Any()
}

// joinKey qualifies key with a group prefix.
func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}

	return prefix + "." + key
}
