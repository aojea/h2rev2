// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package revdial

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

// NewListener returns a new Listener, it dials to the Dialer
// creating "reverse connection" that are accepted by this Listener.
// - client: http client, required for TLS
// - url: path to the reverse handler on the Dialer
// - key: expected key on the Dialer Reverse proxy handler
// - id: identify this listener
func NewListener(client *http.Client, url string, key string, id string) *Listener {

	configureHTTP2Transport(client)

	ln := &Listener{
		url:    url,
		key:    key,
		id:     id,
		client: client,
		connc:  make(chan net.Conn, 4), // arbitrary
		donec:  make(chan struct{}),
	}
	go ln.run()
	return ln
}

var _ net.Listener = (*Listener)(nil)

// Listener is a net.Listener, returning new connections which arrive
// from a corresponding Dialer.
type Listener struct {
	// Request for the reverse connection with format
	// https://url?key=id
	url string
	key string
	id  string

	client *http.Client
	connc  chan net.Conn
	donec  chan struct{}

	mu      sync.Mutex // guards below, closing connc, and writing to rw
	readErr error
	closed  bool
}

// run establish reverse connections against the server
func (ln *Listener) run() {
	defer ln.Close()
	url := "https://" + ln.url + "?" + ln.key + "=" + ln.id
	retry := 0
	// Create connections
	for {
		pr, pw := io.Pipe()
		req, err := http.NewRequest("GET", url, pr)
		if err != nil {
			log.Printf("Can not create request %v", err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")

		log.Printf("Listener creating connection to %s", url)
		res, err := ln.client.Do(req)
		if err != nil {
			retry++
			log.Printf("Can not connect to %s request %v", url, err)
			// TODO: backoff
			time.Sleep(time.Duration(retry*2) * time.Second)
			continue
		}
		retry = 0

		c := NewConn(res.Body, pw)
		select {
		case ln.connc <- c:
		case <-ln.donec:
		}

		select {
		case <-c.Done():
		case <-ln.donec:
		}

		log.Printf("Listener connection to %s closed", url)
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
			return nil, fmt.Errorf("revdial: Listener closed; %v", err)
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

// enable ping to renew connections and avoid issues with stale connections
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
