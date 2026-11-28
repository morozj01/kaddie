// Command certgen writes a throwaway CA and a server certificate for the mock
// OIDC issuer into a directory: ca.crt, tls.crt and tls.key. The
// kube-apiserver of the e2e cluster trusts ca.crt for the issuer.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	out := flag.String("out", ".", "output directory")
	flag.Parse()
	if err := run(*out); err != nil {
		log.Fatal(err)
	}
}

func run(out string) error {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kaddie-test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(30 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "kaddie-oidc"},
		DNSNames:     []string{"kaddie-oidc", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	files := map[string]*pem.Block{
		"ca.crt":  {Type: "CERTIFICATE", Bytes: caDER},
		"tls.crt": {Type: "CERTIFICATE", Bytes: leafDER},
		"tls.key": {Type: "EC PRIVATE KEY", Bytes: keyDER},
	}
	for name, block := range files {
		// 0644: the issuer container runs as an unprivileged user and must read
		// these throwaway test credentials from a bind mount.
		if err := os.WriteFile(filepath.Join(out, name), pem.EncodeToMemory(block), 0o644); err != nil {
			return err
		}
	}
	return nil
}
