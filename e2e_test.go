package h2rev2

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"
)

func Test_e2e(t *testing.T) {
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("backend: revc req %s %s", r.RequestURI, r.RemoteAddr)
		fmt.Fprintf(w, "Hello world")
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()
	defer backend.Close()

	// public server
	dialer := NewDialer()
	defer dialer.Close()
	publicServer := httptest.NewUnstartedServer(dialer)
	publicServer.EnableHTTP2 = true
	publicServer.StartTLS()
	defer publicServer.Close()

	// private server
	l, err := NewListener(backend.Client(), publicServer.URL, "d001")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	mux := http.NewServeMux()
	// reverse proxy queries to an internal host
	proxy, err := NewProxy(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Transport = backend.Client().Transport
	mux.HandleFunc("/", ProxyRequestHandler(proxy))
	server := &http.Server{Handler: mux}
	go server.Serve(l)
	defer server.Close()

	// client
	// wait for the reverse connection to be established
	time.Sleep(1 * time.Second)
	client := publicServer.Client()
	resp, err := client.Get(publicServer.URL + "/proxy/d001/")
	if err != nil {
		t.Fatalf("Request Failed: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Reading body failed: %s", err)
	}
	// Log the request body
	bodyString := string(body)
	if bodyString != "Hello world" {
		t.Errorf("Expected %s received %s", "Hello world", bodyString)
	}
}

// NewProxy takes target host and creates a reverse proxy
func NewProxy(targetHost string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	return proxy, nil
}

// ProxyRequestHandler handles the http request using proxy
func ProxyRequestHandler(proxy *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Reversing proxy to %s", r.URL)
		proxy.ServeHTTP(w, r)
	}
}
