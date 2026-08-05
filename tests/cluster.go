//go:build integration

package tests

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfigv1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// The cluster-facing helpers live behind the integration build tag so client-go
// never gets pulled into a plain `go test ./...`. They read KUBECONFIG, which
// the package init() has already pointed at the cluster HELMTIDE_TEST_CLUSTER
// named -- the same file helm itself uses -- so the tests and the code under
// test always talk to one cluster.

// Dir returns the absolute path of this tests directory, so a plan can name a
// local chart by absolute path regardless of which package's directory the test
// is running from.
func Dir(t *testing.T) string {
	t.Helper()

	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the tests package")
	}

	return filepath.Dir(self)
}

// RESTConfig builds a client config from the isolated KUBECONFIG.
func RESTConfig(t *testing.T) *rest.Config {
	t.Helper()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Fatal("KUBECONFIG is empty: the helpers should have set it")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("build rest config from %s: %s", kubeconfig, err)
	}

	return cfg
}

// Clientset returns a Kubernetes client for the isolated cluster.
func Clientset(t *testing.T) *kubernetes.Clientset {
	t.Helper()

	cs, err := kubernetes.NewForConfig(RESTConfig(t))
	if err != nil {
		t.Fatalf("kubernetes client: %s", err)
	}

	return cs
}

// HelmRevisions maps every stored revision of a release to its status, read
// straight from helm's Secret storage driver (sh.helm.release.v1.<name>.v<N>).
// It is how a test checks that a rollback advanced the revision rather than
// just mutating the live object.
func HelmRevisions(ctx context.Context, t *testing.T, namespace, name string) map[int]string {
	t.Helper()

	cs := Clientset(t)

	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm,name=" + name,
	})
	if err != nil {
		t.Fatalf("list helm release secrets for %s/%s: %s", namespace, name, err)
	}

	revisions := make(map[int]string, len(secrets.Items))

	for i := range secrets.Items {
		labels := secrets.Items[i].GetLabels()

		version, err := strconv.Atoi(labels["version"])
		if err != nil {
			continue
		}

		revisions[version] = labels["status"]
	}

	return revisions
}

// DeployedRevision returns the single revision helm currently marks deployed,
// or 0 if there is none.
func DeployedRevision(ctx context.Context, t *testing.T, namespace, name string) int {
	t.Helper()

	deployed := 0

	for version, status := range HelmRevisions(ctx, t, namespace, name) {
		if status == "deployed" && version > deployed {
			deployed = version
		}
	}

	return deployed
}

// ConfigMap fetches a ConfigMap, failing the test if it cannot be read. The
// full object is returned so callers can inspect data, annotations, and the
// managedFields that server-side apply records.
func ConfigMap(ctx context.Context, t *testing.T, namespace, name string) *corev1.ConfigMap {
	t.Helper()

	cm, err := Clientset(t).CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap %s/%s: %s", namespace, name, err)
	}

	return cm
}

// ConfigMapExists reports whether a ConfigMap is present, so a test can assert a
// hook resource was or was not created without treating NotFound as a failure.
func ConfigMapExists(ctx context.Context, t *testing.T, namespace, name string) bool {
	t.Helper()

	_, err := Clientset(t).CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return true
	}

	if apierrors.IsNotFound(err) {
		return false
	}

	t.Fatalf("get configmap %s/%s: %s", namespace, name, err)

	return false
}

// EnsureNamespace creates a namespace if it is not already there, so a test can
// plant a resource before helm runs.
func EnsureNamespace(ctx context.Context, t *testing.T, namespace string) {
	t.Helper()

	_, err := Clientset(t).CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %s: %s", namespace, err)
	}
}

// ApplyConfigMap server-side applies a ConfigMap under the given field manager,
// so a test can have a foreign manager own a field and then watch helm either
// conflict with it or, with force_conflicts, take it over. The apply error (if
// any) is returned rather than fatal, because a conflict is sometimes the point.
func ApplyConfigMap(
	ctx context.Context,
	t *testing.T,
	namespace, name, manager string,
	data map[string]string,
	force bool,
) error {
	t.Helper()

	ac := applyconfigv1.ConfigMap(name, namespace).WithData(data)

	_, err := Clientset(t).CoreV1().ConfigMaps(namespace).Apply(ctx, ac, metav1.ApplyOptions{
		FieldManager: manager,
		Force:        force,
	})

	return err
}

// FieldManagerFor returns the manager and operation of the managedFields entry
// that owns the ConfigMap's data.value field, so a test can assert whether helm
// wrote it server-side (Apply) or client-side (Update), and under what name.
func FieldManagerFor(
	ctx context.Context,
	t *testing.T,
	namespace, name, manager string,
) (foundManager, operation string, ok bool) {
	t.Helper()

	entries := ConfigMap(ctx, t, namespace, name).GetManagedFields()

	for i := range entries {
		if entries[i].Manager == manager {
			return entries[i].Manager, string(entries[i].Operation), true
		}
	}

	return "", "", false
}

// DeleteNamespace removes a namespace and does not wait for it to disappear.
// Registered as cleanup so a run leaves the cluster as it found it.
func DeleteNamespace(t *testing.T, namespace string) {
	t.Helper()

	cs := Clientset(t)

	err := cs.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("cleanup: delete namespace %s: %s", namespace, err)
	}
}
