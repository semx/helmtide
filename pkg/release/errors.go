package release

import (
	"errors"
	"fmt"

	"github.com/semx/helmtide/pkg/release/uniqname"
	"helm.sh/helm/v4/pkg/storage/driver"
)

var (
	ErrNameEmpty = errors.New("release name is empty")

	// ErrPendingRelease is an error for fail strategy that release is in pending status.
	ErrPendingRelease = errors.New("release is in pending status")

	// ErrValuesNotExist is returned when values can't be used and are skipped.
	ErrValuesNotExist = errors.New("values file doesn't exist")

	// ErrNotFound is an error for not found release.
	ErrNotFound = driver.ErrReleaseNotFound

	// ErrFoundMultiple is an error for multiple releases found by name.
	ErrFoundMultiple = errors.New("found multiple releases o_0")

	// ErrDepFailed is an error thrown when dependency release fails.
	ErrDepFailed = errors.New("dependency failed")

	ErrUnknownFormat = errors.New("unknown format")

	ErrDigestNotMatch = errors.New("chart digest doesn't match")
)

type DuplicateError struct {
	Uniq uniqname.UniqName
}

func NewDuplicateError(uniq uniqname.UniqName) error {
	return &DuplicateError{Uniq: uniq}
}

func (err DuplicateError) Error() string {
	return fmt.Sprintf("release duplicate: %s", err.Uniq.String())
}

type InvalidNamespaceError struct {
	Namespace string
}

func NewInvalidNamespaceError(namespace string) error {
	return &InvalidNamespaceError{Namespace: namespace}
}

func (err InvalidNamespaceError) Error() string {
	return fmt.Sprintf("invalid namespace: %s", err.Namespace)
}

type YAMLDecodeDependsOnError struct {
	Err       error
	DependsOn string
}

func NewYAMLDecodeDependsOnError(dependsOn string, err error) error {
	return &YAMLDecodeDependsOnError{DependsOn: dependsOn, Err: err}
}

func (err YAMLDecodeDependsOnError) Error() string {
	return fmt.Sprintf("failed to decode depends_on reference %q from YAML: %s", err.DependsOn, err.Err)
}

func (err YAMLDecodeDependsOnError) Unwrap() error {
	return err.Err
}

type ChartCacheError struct {
	Err error
}

func NewChartCacheError(err error) error {
	return &ChartCacheError{Err: err}
}

func (err ChartCacheError) Error() string {
	return fmt.Sprintf("failed to find chart in helm cache: %s", err.Err)
}

func (err ChartCacheError) Unwrap() error {
	return err.Err
}

type PostRendererNotFoundError struct {
	Err    error
	Binary string
}

func NewPostRendererNotFoundError(binary string, err error) error {
	return &PostRendererNotFoundError{Binary: binary, Err: err}
}

func (err PostRendererNotFoundError) Error() string {
	return fmt.Sprintf("failed to find post_renderer %q: %s", err.Binary, err.Err)
}

func (err PostRendererNotFoundError) Unwrap() error {
	return err.Err
}

type PostRendererError struct {
	Err    error
	Binary string
	Output string
}

func NewPostRendererError(binary, output string, err error) error {
	return &PostRendererError{Binary: binary, Output: output, Err: err}
}

func (err PostRendererError) Error() string {
	return fmt.Sprintf("post_renderer %q failed: %s\n%s", err.Binary, err.Err, err.Output)
}

func (err PostRendererError) Unwrap() error {
	return err.Err
}

type InvalidWaitStrategyError struct {
	Strategy string
}

func NewInvalidWaitStrategyError(strategy string) error {
	return &InvalidWaitStrategyError{Strategy: strategy}
}

func (err InvalidWaitStrategyError) Error() string {
	return fmt.Sprintf(
		"invalid wait strategy %q, expected a boolean or one of: %s, %s, %s",
		err.Strategy, WaitStrategyWatcher, WaitStrategyLegacy, WaitStrategyHookOnly,
	)
}

// ErrNilRelease is returned when helm hands back no release and no error. Callers dereference the
// result as soon as the error is nil, so a nil release has to be an error rather than a value.
var ErrNilRelease = errors.New("helm returned no release and no error")

type UnexpectedReleaseTypeError struct {
	Release any
}

func NewUnexpectedReleaseTypeError(rel any) error {
	return &UnexpectedReleaseTypeError{Release: rel}
}

func (err UnexpectedReleaseTypeError) Error() string {
	return fmt.Sprintf("helm returned an unsupported release type: %T", err.Release)
}

type HelmTestsError struct {
	Err error
}

func NewHelmTestsError(err error) error {
	return &HelmTestsError{Err: err}
}

func (err HelmTestsError) Error() string {
	return fmt.Sprintf("helm tests failed: %s", err.Err)
}

func (err HelmTestsError) Unwrap() error {
	return err.Err
}

// InvalidServerSideApplyError is returned when server_side_apply is not one of
// "", "true", "false" or "auto".
type InvalidServerSideApplyError struct {
	Value string
}

func NewInvalidServerSideApplyError(v string) error {
	return &InvalidServerSideApplyError{Value: v}
}

func (e InvalidServerSideApplyError) Error() string {
	return fmt.Sprintf("server_side_apply must be \"true\", \"false\" or \"auto\", got %q", e.Value)
}
