package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"

	"github.com/aojea/h2rev2/pkg/revdial"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	// trap Ctrl+C and call cancel on the context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	// Enable signal handler
	signalCh := make(chan os.Signal, 2)
	defer func() {
		close(signalCh)
		cancel()
	}()

	signal.Notify(signalCh, os.Interrupt, unix.SIGINT)
	go func() {
		select {
		case <-signalCh:
			log.Printf("Exiting: received signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	dialer := revdial.NewDialer()
	defer dialer.Close()

	mux := http.NewServeMux()
	mux.Handle("/", dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{Addr: "0.0.0.0:" + port, Handler: mux}
	defer srv.Close()

	log.Printf("Serving on %s", srv.Addr)
	errCh := make(chan error)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = srv.Close()
	}
	if err != nil {
		log.Printf("Exiting with error: %v", err)
	}
}
