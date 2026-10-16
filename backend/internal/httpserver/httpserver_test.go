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