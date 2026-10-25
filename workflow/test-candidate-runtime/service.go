package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tdesaules/github-actions/shared/candidate"
)

func service(root *candidate.Root) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command, err := root.Command(ctx, "/entrypoint")
	if err != nil {
		return err
	}
	var logs bytes.Buffer
	command.Stdout = &logs
	command.Stderr = &logs
	if err = command.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	defer func() {
		cancel()
		<-exited
		if err != nil {
			err = fmt.Errorf("%w: %s", err, logs.String())
		}
	}()
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:5000/ca.pem", nil)
		response, err := client.Do(request)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
			response.Body.Close()
			if response.StatusCode == 200 && readErr == nil {
				pool := x509.NewCertPool()
				if !pool.AppendCertsFromPEM(data) {
					return fmt.Errorf("candidate service returned invalid certificate")
				}
				secure := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
				request, _ = http.NewRequestWithContext(ctx, "GET", "https://127.0.0.1:5443/v2/", nil)
				response, err = secure.Do(request)
				if err == nil {
					response.Body.Close()
					if response.StatusCode == 200 {
						request, _ = http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:5000/v2/", nil)
						response, err = client.Do(request)
						if err == nil {
							response.Body.Close()
							if response.StatusCode == 200 {
								return nil
							}
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("candidate TLS registry did not become ready: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
