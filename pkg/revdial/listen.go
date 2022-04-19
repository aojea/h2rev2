// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package revdial

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"k8s.io/klog/v2"
)

// NewListener returns a new Listener, it dials to the Dialer
// creating "reverse connection" that are accepted by this Listener.
// - client: http client, required for TLS
// - host: a URL to the base of the reverse handler on the Dialer
// - id: identify this listener
func NewListener(client *http.Client, host string, id string) (*Listener, error) {
	err := configureHTTP2Transport(client)
	if err != nil {
		return nil, err
	}

	url, err := serverURL(host, id)
	if err != nil {
		return nil, err
	}

	ln := &Listener{
		url:          url,
		client:       client,
		maxIdleConns: 4,
		connc:        make(chan net.Conn, 4), // arbitrary, TODO define concurrency
		donec:        make(chan struct{}),
	}

	go ln.run()
	return ln, nil
}

var _ net.Listener = (*Listener)(nil)

// Listener is a net.Listener, returning new connections which arrive
// from a corresponding Dialer.
type Listener struct {
	// Request for the reverse connection with format
	// https://host:port/path/revdial?id=<id>
	url string

	client       *http.Client
	maxIdleConns int
	connc        chan net.Conn
	donec        chan struct{}

	mu      sync.Mutex // guards below, closing connc, and writing to rw
	readErr error
	closed  bool
}

// run establish reverse connections against the server
func (ln *Listener) run() {
	defer ln.Close()
	retry := 0
	var mu sync.Mutex
	// Create a pool of connections
	bucket := make(chan struct{}, ln.maxIdleConns)
	for {
		// add a token to the bucket
		select {
		case bucket <- struct{}{}:
		case <-ln.donec:
			return
		}

		go func() {
			// consume the token once finished
			defer func() {
				<-bucket
			}()
			pr, pw := io.Pipe()
			req, err := http.NewRequest("GET", ln.url, pr)
			if err != nil {
				klog.V(5).Infof("Can not create request %v", err)
			}

			klog.V(5).Infof("Listener creating connection to %s", ln.url)
			res, err := ln.client.Do(req)
			if err != nil {
				mu.Lock()
				defer mu.Unlock()
				retry++
				klog.V(5).Infof("Can not connect to %s request %v, retry %d", ln.url, err, retry)
				// TODO: exponential backoff
				time.Sleep(time.Duration(retry*2) * time.Second)
				return
			}
			if res.StatusCode != 200 {
				mu.Lock()
				defer mu.Unlock()
				klog.V(5).Infof("Status code %d on request %v, retry %d", res.StatusCode, ln.url, retry)
				res.Body.Close()
				// TODO: exponential backoff
				time.Sleep(time.Duration(retry*2) * time.Second)
				return
			}
			mu.Lock()
			retry = 0
			mu.Unlock()

			c := NewConn(res.Body, pw)
			defer c.Close()

			select {
			case <-ln.donec:
				return
			default:
				select {
				case ln.connc <- c:
				case <-ln.donec:
					return
				}
			}

			select {
			case <-c.Done():
			case <-ln.donec:
				return
			}
		}()
	}
}

// Closed reports whether the listener has been closed.
func (ln *Listener) Closed() bool {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	return ln.closed
}

// Accept blocks and returns a new connection, or an error.
func (ln *Listener) Accept() (net.Conn, error) {
	c, ok := <-ln.connc
	if !ok {
		ln.mu.Lock()
		err, closed := ln.readErr, ln.closed
		ln.mu.Unlock()
		if err != nil && !closed {
			return nil, fmt.Errorf("revdial: Listener closed; %w", err)
		}
		return nil, ErrListenerClosed
	}
	return c, nil
}

// ErrListenerClosed is returned by Accept after Close has been called.
var ErrListenerClosed = errors.New("revdial: Listener closed")

// Close closes the Listener, making future Accept calls return an
// error.
func (ln *Listener) Close() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	if ln.closed {
		return nil
	}
	ln.closed = true
	close(ln.connc)
	close(ln.donec)
	return nil
}

// Addr returns a dummy address. This exists only to conform to the
// net.Listener interface.
func (ln *Listener) Addr() net.Addr { return connAddr{} }

// configureHTTP2Transport enable ping to avoid issues with stale connections
func configureHTTP2Transport(client *http.Client) error {
	t, ok := client.Transport.(*http.Transport)
	if !ok {
		return nil
	}
	t2, err := http2.ConfigureTransports(t)
	if err != nil {
		return err
	}
	t2.ReadIdleTimeout = time.Duration(30) * time.Second
	t2.PingTimeout = time.Duration(15) * time.Second
	return nil
}

// serverURL builds the destination url with the query parameter
func serverURL(host string, id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("id can not be empty")
	}
	hostURL, err := url.Parse(host)
	if err != nil || hostURL.Scheme != "https" || hostURL.Host == "" {
		return "", fmt.Errorf("wrong url format, expected https://host<:port>/<path>: %w", err)
	}
	host = strings.Trim(host, "/")
	return host + "/" + pathRevDial + "?" + urlParamKey + "=" + id, nil
}
