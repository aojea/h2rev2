// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package revdial

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// The Dialer can create new connections back to the origin.
// A Dialer can have multiple clients.
type Dialer struct {
	incomingConn map[string]chan net.Conn
	connReady    chan bool
	donec        chan struct{}
	closeOnce    sync.Once
}

// NewDialer returns the side of the connection which will initiate
// new connections over the already established reverse connections.
func NewDialer() *Dialer {
	d := &Dialer{
		donec:        make(chan struct{}),
		connReady:    make(chan bool),
		incomingConn: make(map[string]chan net.Conn),
	}
	return d
}

// Done returns a channel which is closed when d is closed (either by
// this process on purpose, by a local error, or close or error from
// the peer).
func (d *Dialer) Done() <-chan struct{} { return d.donec }

// Close closes the Dialer.
func (d *Dialer) Close() error {
	d.closeOnce.Do(d.close)
	return nil
}

func (d *Dialer) close() {
	d.incomingConn = nil
	close(d.donec)
}

// Dial creates a new connection back to the Listener using a reverse tunnel.
// The addr is passed to the dialer is not a real address, is the uniq id that
// identifies the reverse connection.
func (d *Dialer) Dial(ctx context.Context, network string, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(time.Second*5))
	defer cancel()
	log.Printf("Dialing %s %s", network, addr)
	// remove the port added by the std lib
	// the addr is not real, is the id on the incommingConn map
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// pick up one connection:
	select {
	case c := <-d.incomingConn[host]:
		return c, nil
	case <-d.donec:
		return nil, errors.New("revdial.Dialer closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReverseConnectionHandler returns an HTTP handler that establish reverse connections and queue
// them so it can be consumed by the dialer. The Listener from the external clients connects to this
// handler and sends an id as a param identified by the key,
func (d *Dialer) ReverseConnectionHandler(key string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			http.Error(w, "only TLS supported", http.StatusInternalServerError)
			return
		}
		if r.Proto != "HTTP/2.0" {
			http.Error(w, "only HTTP/2.0 supported", http.StatusHTTPVersionNotSupported)
			return
		}
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			http.Error(w, "expected GET request to revdial conn handler", http.StatusMethodNotAllowed)
			return
		}

		// The caller identify itself by the value of the keu
		// https://server/revdial/?key=dialerUniq
		dialerUniq := r.URL.Query().Get(key)
		if len(dialerUniq) == 0 {
			http.Error(w, "only reverse connections with id supported", http.StatusInternalServerError)
			return
		}
		if _, ok := d.incomingConn[dialerUniq]; !ok {
			d.incomingConn[dialerUniq] = make(chan net.Conn)
		}

		log.Printf("created reverse connection to %s %s id %s", r.RequestURI, r.RemoteAddr, dialerUniq)
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		conn := NewConn(r.Body, flushWriter{w})
		select {
		case d.incomingConn[dialerUniq] <- conn:
		case <-d.donec:
			http.Error(w, "Reverse dialer closed", http.StatusInternalServerError)
			return
		}
		// keep the handler alive until the connection is closed
		<-conn.Done()
		log.Printf("Conn from %s done", r.RemoteAddr)
	}
}

// ProxyRequestHandler returns a hander that forwards the request over the reverse connection specified.
// This request is sent directly to the Listener on the other side.
// TODO: check if httputil.ReverseProxy is simpler
// TODO: support copy of request.Body
func (d *Dialer) ProxyRequestHandler(id string) func(http.ResponseWriter, *http.Request) {
	tr := http.DefaultTransport.(*http.Transport)
	tr.DialContext = d.Dial
	client := http.Client{}
	client.Transport = tr
	return func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(context.TODO())
		clone.URL.Host = id
		clone.URL.Scheme = "http"
		clone.RequestURI = ""
		res, err := client.Do(clone)
		if err != nil {
			http.Error(w, "not reverse connection available", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		_, err = io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy server closed %v ", err)
	}
}

type flushWriter struct {
	w io.Writer
}

func (fw flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return
}

func (fw flushWriter) Close() error {
	return nil
}
