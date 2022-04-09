package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/sys/unix"

	"github.com/aojea/h2rev2/pkg/revdial"
)

var (
	flagPublic bool
	flagCert   string
	flagKey    string

	flagDialerPath string
	flagDialerKey  string
	flagDialerID   string

	flagRevProxyPath string
	flagRevProxyHost string
)

func init() {
	flag.BoolVar(&flagPublic, "public", false, "Public mode: forwards to the private server, otherwise connect to the public server")
	flag.StringVar(&flagCert, "cert", "", "Specify the certificate file")
	flag.StringVar(&flagKey, "key", "", "Specify the certificate key file")

	flag.StringVar(&flagDialerPath, "dialer-path", "/revdial", "Specify the dialer path used to dial to")
	flag.StringVar(&flagDialerKey, "dialer-key", "dialerid", "Specify the dialer key used in the URL param")
	flag.StringVar(&flagDialerID, "dialer-id", "001", "Specify the dialer id")

	flag.StringVar(&flagRevProxyPath, "proxy-path", "/", "Specify path where the reverse proxy is listening")
	flag.StringVar(&flagRevProxyHost, "proxy-host", "", "Specify host to reverse proxy")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: h2rev2 [options] [hostname] [port]\n\n"+
			"In private mode, the hostname and port arguments tell what to connect.\n"+
			"In public mode, hostname and port control the address the server will bind to.\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Parse command line flags and arguments
	flag.Parse()
	args := flag.Args()

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

	// expected hostname port
	if len(args) != 2 {
		flag.Usage()
		os.Exit(1)
	}
	addr := net.JoinHostPort(args[0], args[1])

	if flagPublic {
		err := public(ctx, addr)
		if err != nil {
			log.Printf("Error: %v", err)
			os.Exit(1)
		}
	} else {
		err := private(ctx, addr)
		if err != nil {
			log.Printf("Error: %v", err)
			os.Exit(1)
		}
	}
	os.Exit(0)

}

func public(ctx context.Context, addr string) error {
	dialer := revdial.NewDialer(flagKey)
	defer dialer.Close()

	mux := http.NewServeMux()
	// this handler will reverse proxy to the flagDialerID reverse connection
	mux.Handle(flagRevProxyPath, dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{Addr: addr, Handler: mux}
	defer srv.Close()

	log.Printf("Serving on %s", addr)
	errCh := make(chan error)
	go func() {
		errCh <- srv.ListenAndServeTLS(flagCert, flagKey)
	}()
	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = srv.Close()
	}
	return err
}

func private(ctx context.Context, addr string) error {

	client := &http.Client{}
	caCert, err := ioutil.ReadFile(flagCert)
	if err != nil {
		log.Printf("Reading server certificate: %s", err)
		return err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}
	client.Transport = &http2.Transport{
		TLSClientConfig: tlsConfig,
		PingTimeout:     time.Duration(15) * time.Second,
		ReadIdleTimeout: time.Duration(30) * time.Second,
	}

	l := revdial.NewListener(client, addr+flagDialerPath, flagDialerKey, flagDialerID)
	defer l.Close()
	// serve requests
	mux := http.NewServeMux()

	// reverse proxy queries to an internal host
	proxy, err := NewProxy(flagRevProxyHost)
	if err != nil {
		return err
	}
	mux.HandleFunc(flagRevProxyPath, ProxyRequestHandler(proxy))

	server := &http.Server{Handler: mux}
	defer server.Close()

	log.Printf("Serving on Reverse connection")

	errCh := make(chan error)
	go func() {
		errCh <- server.Serve(l)
	}()

	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = server.Close()
	}
	return err
}

// NewProxy takes target host and creates a reverse proxy
func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	log.Printf("Reversing proxy to %s", url)
	proxy := httputil.NewSingleHostReverseProxy(url)
	return proxy, nil
}

// ProxyRequestHandler handles the http request using proxy
func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	}
}
