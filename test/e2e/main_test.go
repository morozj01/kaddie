//go:build e2e

// Package e2e runs end-to-end tests against the kind cluster `make kind-up`
// creates (audit logging on, a mock OIDC issuer trusted). TestMain installs
// the chart; tests that need other values reinstall it and restore the
// defaults when they finish. Tests run one at a time. Run with `make e2e`.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kaddiev1alpha1 "github.com/morozj01/kaddie/pkg/apis/v1alpha1"
	"github.com/morozj01/kaddie/test/internal/testutil"
	"github.com/morozj01/kaddie/test/mocks/oidc"
)

const (
	systemNamespace = "kaddie-system"
	releaseName     = "kaddie"
	// releaseNamespace stores the Helm release record. It can't be
	// kaddie-system, which the chart itself creates and owns.
	releaseNamespace = "default"
	// operatorGroup is bound to kaddie-operator through rbac.operators; the
	// mock issuer's groups claim gets the "oidc:" prefix.
	operatorGroup = "oidc:e2e-operators"
)

var (
	ctx         = context.Background()
	cluster     string
	kubeconfig  string // admin kubeconfig file, for helm and kubectl
	adminConfig *rest.Config
	admin       client.Client
	clientset   *kubernetes.Clientset
	scheme      = runtime.NewScheme()
	oidcInfo    struct {
		Issuer, HostURL, CAFile, ClientID, UsernamePrefix, GroupsPrefix string
	}
)

// defaultValues are the chart values every test starts from.
var defaultValues = []string{
	"--set", "rbac.operators[0].kind=Group",
	"--set", "rbac.operators[0].name=" + operatorGroup,
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	cluster = os.Getenv("KIND_CLUSTER")
	if cluster == "" {
		cluster = "kaddie-e2e"
	}
	if err := setup(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup:", err, "(is the cluster up? run `make kind-up`)")
		return 1
	}
	defer os.Remove(kubeconfig)
	return m.Run()
}

func setup() error {
	for _, add := range []func(*runtime.Scheme) error{clientgoscheme.AddToScheme, kaddiev1alpha1.AddToScheme} {
		if err := add(scheme); err != nil {
			return err
		}
	}
	out, err := exec.Command("kind", "get", "kubeconfig", "--name", cluster).Output()
	if err != nil {
		return fmt.Errorf("kind get kubeconfig: %w", err)
	}
	f, err := os.CreateTemp("", "kaddie-e2e-kubeconfig-*")
	if err != nil {
		return err
	}
	if _, err := f.Write(out); err != nil {
		return err
	}
	f.Close()
	kubeconfig = f.Name()

	adminConfig, err = clientcmd.RESTConfigFromKubeConfig(out)
	if err != nil {
		return err
	}
	if admin, err = client.New(adminConfig, client.Options{Scheme: scheme}); err != nil {
		return err
	}
	if clientset, err = kubernetes.NewForConfig(adminConfig); err != nil {
		return err
	}

	raw, err := os.ReadFile(testutil.Path("test", "infra", ".generated", cluster, "oidc.json"))
	if err != nil {
		return fmt.Errorf("reading the mock issuer's oidc.json: %w", err)
	}
	if err := json.Unmarshal(raw, &oidcInfo); err != nil {
		return err
	}

	// CRDs first: Helm installs crds/ on install only, and a reinstall after a
	// test uninstalls must find them; apply keeps them current.
	if _, err := kubectlE("apply", "--server-side", "-f", testutil.Path("charts", "kaddie", "crds")); err != nil {
		return err
	}
	return installChartE()
}

// --- chart -----------------------------------------------------------------

// installChart (re)installs the chart with the default values plus extra.
func installChart(t *testing.T, extra ...string) {
	t.Helper()
	if err := installChartE(extra...); err != nil {
		t.Fatal(err)
	}
}

func installChartE(extra ...string) error {
	if err := waitNamespaceGone(); err != nil {
		return err
	}
	args := append([]string{"upgrade", "--install", releaseName, testutil.Path("charts", "kaddie"),
		"--namespace", releaseNamespace, "--wait", "--timeout", "2m"}, defaultValues...)
	if _, err := helmE(append(args, extra...)...); err != nil {
		return err
	}
	return waitQuotaReady()
}

// waitQuotaReady waits until the quota controller has computed the
// proposal quota's status. Until then the API server refuses every
// SREProposal create with "status unknown for quota". On a fresh cluster
// that takes over a minute: the controller first has to discover the
// just-installed sreproposals type, on its own discovery resync.
func waitQuotaReady() error {
	start := time.Now()
	var last corev1.ResourceQuotaStatus
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		q := &corev1.ResourceQuota{}
		if err := admin.Get(ctx, client.ObjectKey{Namespace: systemNamespace, Name: "kaddie-proposals"}, q); err != nil {
			return false, client.IgnoreNotFound(err)
		}
		last = q.Status
		res := corev1.ResourceName("count/sreproposals.kaddie.org")
		_, used := q.Status.Used[res]
		return used && q.Status.Hard[res].Equal(q.Spec.Hard[res]), nil
	})
	if err != nil {
		return fmt.Errorf("waiting for the proposal quota's status (last seen hard=%v used=%v): %w", last.Hard, last.Used, err)
	}
	if d := time.Since(start); d > 5*time.Second {
		fmt.Fprintf(os.Stderr, "e2e: the proposal quota's status took %s to appear\n", d.Round(time.Second))
	}
	return nil
}

// restoreChart reinstalls the default values when the test ends.
func restoreChart(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := installChartE(); err != nil {
			t.Errorf("restoring the default chart: %v", err)
		}
	})
}

func uninstallChart(t *testing.T) {
	t.Helper()
	if _, err := helmE("uninstall", releaseName, "--namespace", releaseNamespace, "--wait"); err != nil {
		t.Fatal(err)
	}
	if err := waitNamespaceGone(); err != nil {
		t.Fatal(err)
	}
}

// waitNamespaceGone waits until kaddie-system isn't terminating, so a
// reinstall after an uninstall can create it again.
func waitNamespaceGone() error {
	err := wait.PollUntilContextTimeout(ctx, time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		ns := &corev1.Namespace{}
		err := admin.Get(ctx, client.ObjectKey{Name: systemNamespace}, ns)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return ns.DeletionTimestamp == nil, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for namespace %s to finish terminating: %w", systemNamespace, err)
	}
	return nil
}

func helm(t *testing.T, args ...string) string {
	t.Helper()
	out, err := helmE(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func helmE(args ...string) (string, error) {
	return runCmd("helm", append(args, "--kubeconfig", kubeconfig)...)
}

func kubectlE(args ...string) (string, error) {
	return runCmd("kubectl", append([]string{"--kubeconfig", kubeconfig}, args...)...)
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// --- identities ------------------------------------------------------------

// saName is the username of a kaddie-system ServiceAccount.
func saName(sa string) string { return "system:serviceaccount:" + systemNamespace + ":" + sa }

// asServiceAccount returns a client authenticated with a real, short-lived
// token for the ServiceAccount, so the API server and the audit log see the
// ServiceAccount itself rather than an impersonation.
func asServiceAccount(t *testing.T, sa string) client.Client {
	t.Helper()
	return tokenClient(t, saToken(t, systemNamespace, sa))
}

// saToken requests a ten-minute token for a ServiceAccount (TokenRequest).
func saToken(t *testing.T, namespace, sa string) string {
	t.Helper()
	exp := int64(600)
	tr, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, sa,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &exp}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("token for %s/%s: %v", namespace, sa, err)
	}
	return tr.Status.Token
}

// oidcToken mints an ID token from the mock issuer for email and groups.
func oidcToken(t *testing.T, email string, groups ...string) string {
	t.Helper()
	body, _ := json.Marshal(oidc.TokenRequest{Subject: email, Email: email, Groups: groups, Audience: oidcInfo.ClientID})
	resp, err := oidcHTTP(t).Post(oidcInfo.HostURL+oidc.TokenPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("minting a token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("minting a token: %s", resp.Status)
	}
	var tr oidc.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatal(err)
	}
	return tr.IDToken
}

// asHuman returns a client authenticated as an OIDC user. Group names are
// given without the "oidc:" prefix the API server adds.
func asHuman(t *testing.T, email string, groups ...string) client.Client {
	t.Helper()
	return tokenClient(t, oidcToken(t, email, groups...))
}

// operator is a human in the group the chart binds to kaddie-operator.
func operator(t *testing.T) client.Client {
	return asHuman(t, "operator@example.com", strings.TrimPrefix(operatorGroup, "oidc:"))
}

func tokenConfig(token string) *rest.Config {
	cfg := rest.AnonymousClientConfig(adminConfig)
	cfg.BearerToken = token
	return cfg
}

func tokenClient(t *testing.T, token string) client.Client {
	t.Helper()
	c, err := client.New(tokenConfig(token), client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func oidcHTTP(t *testing.T) *http.Client {
	t.Helper()
	cfg := &rest.Config{TLSClientConfig: rest.TLSClientConfig{CAFile: filepath.FromSlash(oidcInfo.CAFile)}}
	tr, err := rest.TransportFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}
