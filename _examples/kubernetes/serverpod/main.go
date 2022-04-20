package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/aojea/h2rev2"
	"golang.org/x/sys/unix"
)

var (
	flagPort     string
	flagCert     string
	flagKey      string
	flagBasePath string
)

func init() {
	flag.StringVar(&flagPort, "port", "8080", "Specify the default port to listen")
	flag.StringVar(&flagCert, "cert", "", "Specify the server certificate file")
	flag.StringVar(&flagKey, "key", "", "Specify the server certificate key file")
	flag.StringVar(&flagBasePath, "base-path", "/", "Specify the base-path the reverse dialer handler should use")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: h2rev2server [options]\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Parse command line flags and arguments
	flag.Parse()

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

	dialer := h2rev2.NewDialer()
	defer dialer.Close()

	mux := http.NewServeMux()
	mux.Handle(flagBasePath, dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{
		Addr:    "0.0.0.0:" + flagPort,
		Handler: mux,
	}
	defer srv.Close()

	log.Printf("Serving on %s", srv.Addr)
	errCh := make(chan error)
	go func() {
		if flagCert != "" && flagKey != "" {
			errCh <- srv.ListenAndServeTLS(flagCert, flagKey)
		} else {
			errCh <- fmt.Errorf("not certificate provided")
		}
	}()
	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = srv.Close()
	}
	if err != nil {
		log.Printf("Exiting with error: %v", err)
		os.Exit(1)
	}
}
