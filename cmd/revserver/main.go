package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sys/unix"

	"github.com/aojea/h2rev2/pkg/revdial"
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

	dialer := revdial.NewDialer()
	defer dialer.Close()

	mux := http.NewServeMux()
	mux.Handle(flagBasePath, dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{
		Addr:    "0.0.0.0:" + flagPort,
		Handler: mux,
		TLSConfig: &tls.Config{
			NextProtos: []string{"h2"},
		},
	}
	defer srv.Close()

	log.Printf("Serving on %s", srv.Addr)
	errCh := make(chan error)
	go func() {
		if flagCert != "" && flagKey != "" {
			errCh <- srv.ListenAndServeTLS(flagCert, flagKey)
		} else {
			certManager := autocert.Manager{
				Prompt: autocert.AcceptTOS,
				Cache:  autocert.DirCache("certs"),
			}
			srv.TLSConfig = &tls.Config{GetCertificate: certManager.GetCertificate}
			errCh <- srv.ListenAndServe()
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
	}
}
