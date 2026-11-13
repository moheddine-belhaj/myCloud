package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}

	done := make(chan error, 1)
	go func() { done <- Run(ctx, srv, time.Second) }()

	time.Sleep(50 * time.Millisecond) // let the server start listening
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	srv := &http.Server{Addr: "not-a-valid-address"}
	if err := Run(context.Background(), srv, time.Second); err == nil {
		t.Fatal("Run with an invalid address returned nil")
	}
}

func TestRunAllStopsEveryServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	b := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}

	done := make(chan error, 1)
	go func() { done <- RunAll(ctx, time.Second, a, b) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAll returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunAll did not return after the context was cancelled")
	}
}

// One listener failing (port taken, bad address) must stop the healthy one,
// so the pod restarts instead of running without its internal endpoint.
func TestRunAllStopsOthersWhenOneFails(t *testing.T) {
	ok := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	bad := &http.Server{Addr: "not-a-valid-address"}

	done := make(chan error, 1)
	go func() { done <- RunAll(context.Background(), time.Second, ok, bad) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunAll returned nil, want the listen error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunAll kept running after one server failed")
	}
}
