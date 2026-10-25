package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tdesaules/github-actions/shared/workflow"
)

func certificate() (cert, key []byte, err error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "test-registry"}, DNSNames: []string{"test-registry", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded}), nil
}
func run() error {
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	dir, err := os.MkdirTemp("", "test-registry-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cert, key, err := certificate()
	if err != nil {
		return err
	}
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err = os.WriteFile(certPath, cert, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(keyPath, key, 0600); err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return err
	}
	config, _ := json.Marshal(map[string]any{"version": "0.1", "log": map[string]string{"level": "error"}, "storage": map[string]any{"filesystem": map[string]string{"rootdirectory": filepath.Join(dir, "data")}}, "http": map[string]any{"addr": ":5443", "relativeurls": true, "secret": fmt.Sprintf("%x", secret), "tls": map[string]string{"certificate": certPath, "key": keyPath}}})
	configPath := filepath.Join(dir, "config.json")
	if err = os.WriteFile(configPath, config, 0600); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/registry", "serve", configPath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err = command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	mux := http.NewServeMux()
	// The candidate-registry role exposes only a private job/loopback transport.
	// Public publication tests use the TLS listener directly, never this proxy.
	target, _ := url.Parse("https://127.0.0.1:5443")
	proxy := httputil.NewSingleHostReverseProxy(target)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cert)
	proxy.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	mux.Handle("/v2/", proxy)
	mux.HandleFunc("/ca.pem", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Write(cert)
	})
	server := &http.Server{Addr: ":5000", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	serving := make(chan error, 1)
	go func() { serving <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		<-exited
		return nil
	case err := <-exited:
		return fmt.Errorf("registry stopped unexpectedly: %v", err)
	case err := <-serving:
		cancel()
		<-exited
		return err
	}
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "--tool-version" {
		command := exec.Command("/usr/bin/registry", "--version")
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	workflow.Main("start-test-registry 1", run)
}
