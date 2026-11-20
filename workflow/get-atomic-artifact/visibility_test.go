package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func visibilityClient(t *testing.T, o options, snapshots [][]artifact) (*http.Client, *int) {
	t.Helper()
	lookups := 0
	return &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Fatal("unexpected request identity")
		}
		var data any
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			if r.URL.Query().Get("page") != "1" {
				t.Fatal("visibility lookup did not restart pagination")
			}
			index := lookups
			lookups++
			if index >= len(snapshots) {
				index = len(snapshots) - 1
			}
			data = map[string]any{"total_count": len(snapshots[index]), "artifacts": snapshots[index]}
		} else {
			data = map[string]any{"id": 42, "head_sha": o.SHA, "run_attempt": 3, "repository": map[string]string{"full_name": o.Repository}}
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}, &lookups
}

func TestOnlyKnownSuccessfulUploadGetsVisibilityRetries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expected  string
		snapshots [][]artifact
		wantID    int64
		lookups   int
		waits     int
		wantError string
	}{
		{"delayed upload", "103", [][]artifact{nil, {item(3, false)}}, 103, 2, 1, ""},
		{"older visible does not replace known upload", "103", [][]artifact{{item(2, false)}, {item(2, false), item(3, false)}}, 103, 2, 1, ""},
		{"bounded absent upload", "103", [][]artifact{nil}, 0, 6, 5, "not visible after 6"},
		{"no producer proof means no retry", "", [][]artifact{nil, {item(3, false)}}, 0, 1, 0, "no matching"},
		{"ordinary rerun still selects older success", "", [][]artifact{{item(2, false)}}, 102, 1, 0, ""},
		{"expired newest never falls back or retries", "103", [][]artifact{{item(2, false), item(3, true)}}, 0, 1, 0, "expired"},
		{"duplicate attempt never retries", "103", [][]artifact{{item(3, false), item(3, false)}}, 0, 1, 0, "ambiguous"},
		{"foreign expected name never retries", "999", [][]artifact{{{ID: 999, Name: "candidate-os-arm64-42-3"}}}, 0, 1, 0, "wrong identity"},
		{"future expected upload never retries", "104", [][]artifact{{item(4, false)}}, 0, 1, 0, "exceeds current"},
		{"older producer proof cannot override newer selection", "102", [][]artifact{{item(2, false), item(3, false)}}, 0, 1, 0, "not the newest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := fixture()
			o.ExpectedID = tc.expected
			client, lookups := visibilityClient(t, o, tc.snapshots)
			delays := []time.Duration{}
			got, err := selectVisibleArtifact(context.Background(), client, "secret-token", o, func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil // Virtual backoff; unit tests never sleep.
			})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("wanted %q, got %v", tc.wantError, err)
				}
			} else if err != nil || got.ID != tc.wantID {
				t.Fatalf("unexpected selected artifact %+v: %v", got, err)
			}
			if *lookups != tc.lookups || len(delays) != tc.waits {
				t.Fatalf("lookups=%d delays=%v", *lookups, delays)
			}
			for i, delay := range delays {
				if delay != time.Second<<i {
					t.Fatalf("unexpected bounded backoff: %v", delays)
				}
			}
		})
	}
}

func TestVisibilityDoesNotRetryIdentityOrHTTPFailure(t *testing.T) {
	o := fixture()
	o.ExpectedID = "103"
	for _, status := range []int{200, 401, 403, 404, 500} {
		calls, waits := 0, 0
		client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			// Even a successful HTTP response with a different run SHA is fatal.
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"id":42,"head_sha":"wrong","run_attempt":3,"repository":{"full_name":"owner/repo"}}`))}, nil
		})}
		if _, err := selectVisibleArtifact(context.Background(), client, "secret-token", o, func(context.Context, time.Duration) error {
			waits++
			return nil
		}); err == nil || calls != 1 || waits != 0 {
			t.Fatalf("status=%d: error=%v calls=%d waits=%d", status, err, calls, waits)
		}
	}
}

func TestVisibilityBackoffHonorsCancellation(t *testing.T) {
	o := fixture()
	o.ExpectedID = "103"
	client, lookups := visibilityClient(t, o, [][]artifact{nil, {item(3, false)}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := selectVisibleArtifact(ctx, client, "secret-token", o, func(ctx context.Context, _ time.Duration) error {
		cancel()
		return waitForVisibility(ctx, time.Hour) // Already canceled: no wall-clock sleep.
	})
	if !errors.Is(err, context.Canceled) || *lookups != 1 {
		t.Fatalf("cancellation lost: %v lookups=%d", err, *lookups)
	}
}

func TestExpectedProducerIDValidation(t *testing.T) {
	for _, id := range []string{"0", "001", "-1", "not-an-id", "9223372036854775808"} {
		o := fixture()
		o.ExpectedID = id
		env := envFor(o)
		if err := validate(o, func(k string) string { return env[k] }); err == nil {
			t.Fatalf("invalid producer ID accepted: %q", id)
		}
	}
}

func TestChangedListingRestartsFromFirstPage(t *testing.T) {
	o := fixture()
	o.ExpectedID = "103"
	calls := 0
	waits := 0
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		var data any
		switch calls {
		case 1, 4:
			if strings.HasSuffix(r.URL.Path, "/artifacts") {
				t.Fatal("run identity not rechecked")
			}
			data = map[string]any{"id": 42, "head_sha": o.SHA, "run_attempt": 3, "repository": map[string]string{"full_name": o.Repository}}
		case 2:
			if r.URL.Query().Get("page") != "1" {
				t.Fatal("expected page one")
			}
			data = map[string]any{"total_count": 101, "artifacts": make([]artifact, 100)}
		case 3:
			if r.URL.Query().Get("page") != "2" {
				t.Fatal("expected page two")
			}
			data = map[string]any{"total_count": 102, "artifacts": []artifact{item(3, false)}}
		case 5:
			if r.URL.Query().Get("page") != "1" {
				t.Fatal("reused inconsistent page")
			}
			data = map[string]any{"total_count": 1, "artifacts": []artifact{item(3, false)}}
		default:
			t.Fatal("unexpected request")
		}
		b, _ := json.Marshal(data)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	})}
	got, err := selectVisibleArtifact(context.Background(), client, "secret-token", o, func(context.Context, time.Duration) error { waits++; return nil })
	if err != nil || got.ID != 103 || calls != 5 || waits != 1 {
		t.Fatalf("got=%v err=%v calls=%d waits=%d", got, err, calls, waits)
	}
}
