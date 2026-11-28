//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TestOIDC_Spike is implementation plan T0.3: a token from the mock issuer
// authenticates to the kind cluster, and the API server applies the
// configured username and group prefixes, as SelfSubjectReview reports both
// through client-go and through `kubectl auth whoami --token`. HT15 and UT3
// build on this.
func TestOIDC_Spike(t *testing.T) {
	token := oidcToken(t, "alice@example.com", "devs", "sre")
	wantUser := oidcInfo.UsernamePrefix + "alice@example.com"
	wantGroups := []string{oidcInfo.GroupsPrefix + "devs", oidcInfo.GroupsPrefix + "sre", "system:authenticated"}

	cs, err := kubernetes.NewForConfig(tokenConfig(token))
	if err != nil {
		t.Fatal(err)
	}
	review, err := cs.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("SelfSubjectReview with the issuer's token: %v", err)
	}
	checkUser(t, "client-go", review.Status.UserInfo.Username, review.Status.UserInfo.Groups, wantUser, wantGroups)

	// The same through kubectl, with a kubeconfig holding only the token.
	cfg, err := clientcmd.Load(mustReadFile(t, kubeconfig))
	if err != nil {
		t.Fatal(err)
	}
	cfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{"oidc": {Token: token}}
	for name, c := range cfg.Contexts {
		c.AuthInfo = "oidc"
		cfg.CurrentContext = name
	}
	f, err := os.CreateTemp(t.TempDir(), "kubeconfig-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := clientcmd.WriteToFile(*cfg, f.Name()); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd("kubectl", "--kubeconfig", f.Name(), "auth", "whoami", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var whoami authenticationv1.SelfSubjectReview
	if err := json.Unmarshal([]byte(out), &whoami); err != nil {
		t.Fatalf("kubectl auth whoami output: %v\n%s", err, out)
	}
	checkUser(t, "kubectl", whoami.Status.UserInfo.Username, whoami.Status.UserInfo.Groups, wantUser, wantGroups)

	// A token for another audience is rejected.
	if _, err := kubernetes.NewForConfigOrDie(tokenConfig(token+"x")).AuthenticationV1().SelfSubjectReviews().
		Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{}); err == nil {
		t.Error("a tampered token authenticated")
	}
}

func checkUser(t *testing.T, via, user string, groups []string, wantUser string, wantGroups []string) {
	t.Helper()
	if user != wantUser {
		t.Errorf("%s: username = %q, want %q", via, user, wantUser)
	}
	for _, g := range wantGroups {
		if !slices.Contains(groups, g) {
			t.Errorf("%s: groups %v lack %q", via, groups, g)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
