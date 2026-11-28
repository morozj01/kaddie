// Command oidc-issuer serves the mock OIDC issuer over TLS. test/infra runs
// it as a container on the kind network so the kube-apiserver can fetch its
// discovery document and keys.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/morozj01/kaddie/test/mocks/oidc"
)

func main() {
	issuerURL := flag.String("issuer", "https://kaddie-oidc:8443", "issuer URL placed in tokens and discovery")
	listen := flag.String("listen", ":8443", "listen address")
	cert := flag.String("tls-cert", "/certs/tls.crt", "TLS certificate")
	key := flag.String("tls-key", "/certs/tls.key", "TLS key")
	flag.Parse()

	iss, err := oidc.New(*issuerURL)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: *listen, Handler: iss.Handler(), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("mock OIDC issuer %s listening on %s", *issuerURL, *listen)
	log.Fatal(srv.ListenAndServeTLS(*cert, *key))
}
