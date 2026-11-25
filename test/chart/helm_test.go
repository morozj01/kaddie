// Package chart holds render tests for charts/kaddie: `helm template` is run
// with chosen values and the output is checked against the spec. Run with
// `make chart-test`.
package chart

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/morozj01/kaddie/test/internal/testutil"
)

// minKube is the minimum supported Kubernetes version (base §13).
const minKube = "1.28.0"

var chartDir = testutil.Path("charts", "kaddie")

// render runs helm template on dir with the given extra arguments and
// returns the objects, failing the test on a render error.
func render(t *testing.T, dir string, args ...string) []*unstructured.Unstructured {
	t.Helper()
	out, err := helmTemplate(dir, args...)
	if err != nil {
		t.Fatalf("helm template %v: %v", args, err)
	}
	return decode(t, out)
}

func helmTemplate(dir string, args ...string) ([]byte, error) {
	full := append([]string{"template", "kaddie", dir, "--kube-version", minKube}, args...)
	// A later --kube-version wins, so callers can override the default.
	cmd := exec.Command("helm", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, errors.New(strings.TrimSpace(stderr.String()) + ": " + err.Error())
	}
	return stdout.Bytes(), nil
}

func decode(t *testing.T, manifest []byte) []*unstructured.Unstructured {
	t.Helper()
	dec := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	var objs []*unstructured.Unstructured
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			if errors.Is(err, io.EOF) {
				return objs
			}
			t.Fatalf("decode manifest: %v", err)
		}
		if len(m) == 0 {
			continue
		}
		objs = append(objs, &unstructured.Unstructured{Object: m})
	}
}

// find returns the rendered object with this kind and name, or nil.
func find(objs []*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	for _, o := range objs {
		if o.GetKind() == kind && o.GetName() == name {
			return o
		}
	}
	return nil
}

func ofKind(objs []*unstructured.Unstructured, kinds ...string) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, o := range objs {
		for _, k := range kinds {
			if o.GetKind() == k {
				out = append(out, o)
			}
		}
	}
	return out
}

// sample loads one object from test/fixtures/yaml-samples.
func sample(t *testing.T, elem ...string) *unstructured.Unstructured {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(append([]string{testutil.Path("test", "fixtures", "yaml-samples")}, elem...)...))
	if err != nil {
		t.Fatal(err)
	}
	o := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, &o.Object); err != nil {
		t.Fatal(err)
	}
	return o
}

// gitopsModes runs f with GitOps off and on.
func gitopsModes(t *testing.T, f func(t *testing.T, gitops bool, args []string)) {
	for _, gitops := range []bool{false, true} {
		name, args := "gitops-off", []string{}
		if gitops {
			name, args = "gitops-on", []string{"--set", "gitops.enabled=true"}
		}
		t.Run(name, func(t *testing.T) { f(t, gitops, args) })
	}
}
