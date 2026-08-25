package httptransport

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"bioacoustic-release-hub/internal/application"
	"bioacoustic-release-hub/internal/store/sqlite"
)

func TestFullSelfCheck(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: NewHandler(application.NewService(store))}
	go server.Serve(listener)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := RunSelfCheck(ctx, "http://"+listener.Addr().String()); err != nil {
		t.Fatal(err)
	}
	_ = server.Shutdown(ctx)
}
