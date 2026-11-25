//go:build envtest

// Package envtest runs CRD and CEL behaviour tests against a real
// kube-apiserver (and etcd) at the minimum supported version, 1.28, started
// by controller-runtime's envtest. Run with `make test-envtest`.
package envtest

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	kaddiev1alpha1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/test/internal/testutil"
)

const systemNamespace = "kaddie-system"

var (
	cfg    *rest.Config
	k8s    client.Client
	scheme = runtime.NewScheme()
	ctx    = context.Background()
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := kaddiev1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{testutil.Path("charts", "kaddie", "crds")},
		ErrorIfCRDPathMissing: true,
	}
	var err error
	cfg, err = env.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "envtest: start:", err, "(run through `make test-envtest`, which sets KUBEBUILDER_ASSETS)")
		return 1
	}
	defer func() {
		if err := env.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "envtest: stop:", err)
		}
	}()

	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintln(os.Stderr, "envtest: client:", err)
		return 1
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: systemNamespace}}
	if err := k8s.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		fmt.Fprintln(os.Stderr, "envtest: namespace:", err)
		return 1
	}
	return m.Run()
}

// TestEnvtest_ServerIsMinimumVersion proves the suite runs against the
// minimum supported Kubernetes release (T0.2).
func TestEnvtest_ServerIsMinimumVersion(t *testing.T) {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	v, err := dc.ServerVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v.Major != "1" || v.Minor != "28" {
		t.Fatalf("envtest kube-apiserver is %s.%s, want 1.28", v.Major, v.Minor)
	}
}
