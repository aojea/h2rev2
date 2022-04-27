package h2rev2

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"
)

func setup(t *testing.T) (*http.Client, string, func()) {
	t.Helper()
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello world")
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()

	// public server
	pool := NewReversePool()
	publicServer := httptest.NewUnstartedServer(pool)
	publicServer.EnableHTTP2 = true
	publicServer.StartTLS()

	// private server
	l, err := NewListener(publicServer.Client(), publicServer.URL, "d001")
	if err != nil {
		t.Fatal(err)
	}

	// reverse proxy queries to an internal host
	url, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = backend.Client().Transport
	server := &http.Server{Handler: proxy}
	go server.Serve(l)

	// client
	// wait for the reverse connection to be established
	time.Sleep(1 * time.Second)
	stop := func() {
		l.Close()
		server.Close()
		publicServer.Close()
		backend.Close()
	}
	return publicServer.Client(), publicServer.URL + "/proxy/d001/", stop

}

func Test_e2e(t *testing.T) {
	client, uri, stop := setup(t)
	defer stop()
	resp, err := client.Get(uri)
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

func Test_e2e_multiple_connections(t *testing.T) {
	client, uri, stop := setup(t)
	defer stop()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(uri)
			if err != nil {
				t.Errorf("Request Failed: %s", err)
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("Reading body failed: %s", err)
			}
			// Log the request body
			bodyString := string(body)
			if bodyString != "Hello world" {
				t.Errorf("Expected %s received %s", "Hello world", bodyString)
			}
		}()
	}
	wg.Wait()
}
