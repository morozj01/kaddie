// Command vendor-openapi records the OpenAPI v3 schemas pkg/proposal/schema
// serves offline. It starts a kube-apiserver through controller-runtime's
// envtest (KUBEBUILDER_ASSETS names the binaries, as for `make test-envtest`),
// records the workload kinds through schema.Record, the code path schema.Live
// takes, and writes the files into the schema package's openapi directory,
// removing any file it no longer produces. It is idempotent: against the same
// server it rewrites identical bytes.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/morozj01/kaddie/pkg/proposal/schema"
)

// resources are the workload kinds the built-in field rule covers (base
// §6.3). Record adds their subresources.
var resources = []schema.Resource{
	{Version: "v1", Resource: "pods"},
	{Version: "v1", Resource: "replicationcontrollers"},
	{Group: "apps", Version: "v1", Resource: "deployments"},
	{Group: "apps", Version: "v1", Resource: "replicasets"},
	{Group: "apps", Version: "v1", Resource: "statefulsets"},
	{Group: "apps", Version: "v1", Resource: "daemonsets"},
	{Group: "batch", Version: "v1", Resource: "jobs"},
	{Group: "batch", Version: "v1", Resource: "cronjobs"},
}

func main() {
	out := flag.String("out", filepath.Join("pkg", "proposal", "schema", "openapi"), "directory to write the files into")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "vendor-openapi:", err)
		os.Exit(1)
	}
}

func run(out string) (err error) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		return errors.New("KUBEBUILDER_ASSETS is not set (run through `make vendor-openapi`)")
	}
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		return fmt.Errorf("start envtest: %w", err)
	}
	defer func() { err = errors.Join(err, env.Stop()) }()
	d, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}

	// The server publishes its OpenAPI v3 documents a moment after it starts.
	var files map[string][]byte
	deadline := time.Now().Add(time.Minute)
	for {
		files, err = schema.Record(d, resources)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("record: %w", err)
		}
		time.Sleep(time.Second)
	}
	return write(out, files)
}

// write makes dir hold exactly files, touching only the files that differ.
func write(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	existing, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range existing {
		if _, keep := files[filepath.Base(path)]; !keep {
			if err := os.Remove(path); err != nil {
				return err
			}
			fmt.Println("removed", path)
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, files[name]) {
			fmt.Println("unchanged", path)
			continue
		}
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path, len(files[name]), "bytes")
	}
	return nil
}
