package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"

	"golang.org/x/net/http2"
	"golang.org/x/sys/unix"

	"github.com/aojea/h2rev2/pkg/revdial"
)

var (
	flagURL          string
	flagRevProxyHost string
	flagRevProxyCert string
	flagCert         string
	flagDialerID     string
)

func init() {
	flag.StringVar(&flagURL, "url", "", "Specify the server broker url")
	flag.StringVar(&flagCert, "cert", "", "Specify the server certificate file")
	flag.StringVar(&flagDialerID, "dialer-id", "", "Specify the dialer id (default: hostname")
	flag.StringVar(&flagRevProxyHost, "proxy-host", "", "Specify host to reverse proxy")
	flag.StringVar(&flagRevProxyCert, "proxy-host-cert", "", "Specify cert file name for the host to reverse proxy")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: h2rev2client [options]\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Parse command line flags and arguments
	flag.Parse()
	var err error

	if flagURL == "" {
		panic(fmt.Errorf("empty url"))
	}

	if flagDialerID == "" {
		flagDialerID, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}

	if flagRevProxyHost == "" {
		panic(fmt.Errorf("empty host to reverse proxy"))

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

	client := &http.Client{}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	if flagCert != "" {
		caCert, err := ioutil.ReadFile(flagCert)
		if err != nil {
			log.Fatalf("Reading server certificate: %s", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: false,
		}
	}
	client.Transport = &http2.Transport{
		TLSClientConfig: tlsConfig,
	}

	// initialize a reverse proxy and pass the actual backend server url here
	proxy, err := NewProxy(flagRevProxyHost)
	if err != nil {
		panic(err)
	}
	trProxy := http.DefaultTransport.(*http.Transport)
	trProxy.TLSClientConfig.InsecureSkipVerify = true

	if flagRevProxyCert != "" {
		caCert, err := ioutil.ReadFile(flagRevProxyCert)
		if err != nil {
			log.Fatalf("Reading server certificate: %s", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: false,
		}
	}

	l, err := revdial.NewListener(client, flagURL, flagDialerID)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	// serve requests
	mux := http.NewServeMux()
	mux.HandleFunc("/", ProxyRequestHandler(proxy))
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
	if err != nil {
		log.Printf("error shutting down: %v", err)
		os.Exit(1)
	}
	os.Exit(0)

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
