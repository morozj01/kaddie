//go:build envtest

package envtest

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/client-go/discovery"

	"github.com/morozj01/kaddie/pkg/proposal/schema"
	"github.com/morozj01/kaddie/test/internal/testutil"
)

// TestSchema_LiveMatchesVendored proves the schemas under
// pkg/proposal/schema/openapi are what a 1.28 kube-apiserver serves, read
// through the same code Live uses. Re-recording every vendored resource must
// rebuild the vendored files byte for byte; when the server or the recorder
// changes, `make vendor-openapi` is the fix. The control keeps that
// comparison able to fail: a resource outside the vendor index is unknown to
// Vendored but known to Live.
func TestSchema_LiveMatchesVendored(t *testing.T) {
	d, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	live, vendored := schema.Live(d), schema.Vendored()

	var tops []schema.Resource
	for _, r := range schema.Recorded() {
		if r.Subresource == "" {
			tops = append(tops, r)
		}
	}

	// The server publishes its OpenAPI v3 documents a moment after it starts.
	var recorded map[string][]byte
	deadline := time.Now().Add(time.Minute)
	for {
		recorded, err = schema.Record(d, tops)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recording the vendored resources from the live server: %v", err)
		}
		time.Sleep(time.Second)
	}

	dir := testutil.Path("pkg", "proposal", "schema", "openapi")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var vendoredNames []string
	for _, e := range entries {
		vendoredNames = append(vendoredNames, e.Name())
	}
	var liveNames []string
	for name := range recorded {
		liveNames = append(liveNames, name)
	}
	sort.Strings(vendoredNames)
	sort.Strings(liveNames)
	if diff := cmp.Diff(vendoredNames, liveNames); diff != "" {
		t.Fatalf("vendored files differ from the live recording (-vendored +live):\n%s", diff)
	}
	for name, got := range recorded {
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(string(want), string(got)); diff != "" {
			t.Errorf("%s: the live document differs from the vendored one (-vendored +live):\n%s", name, diff)
		}
	}

	for _, r := range schema.Recorded() {
		if !vendored.Root(r).Known() {
			t.Errorf("vendored provider does not know recorded resource %+v", r)
		}
		if !live.Root(r).Known() {
			t.Errorf("live provider does not know vendored resource %+v", r)
		}
	}
	scale := live.Root(schema.Resource{Group: "apps", Version: "v1", Resource: "deployments", Subresource: "scale"})
	if !scale.Field("spec").Field("replicas").Known() || scale.Field("spec").Field("template").Known() {
		t.Error("live deployments/scale is not the Scale schema")
	}

	control := schema.Resource{Group: "apps", Version: "v1", Resource: "controllerrevisions"}
	for _, r := range schema.Recorded() {
		if r == control {
			t.Fatalf("control %+v is vendored, so it no longer proves the comparison can fail", control)
		}
	}
	if vendored.Root(control).Known() {
		t.Errorf("control %+v is known to the vendored provider", control)
	}
	if !live.Root(control).Known() {
		t.Errorf("control %+v is unknown to the live provider", control)
	}
}
