// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package revdial

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The Dialer can create new connections back to the origin.
// A Dialer can have multiple clients.
type Dialer struct {
	key          string
	incomingConn map[string]chan net.Conn
	connReady    chan bool
	donec        chan struct{}
	closeOnce    sync.Once
	revClient    *http.Client
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

// reverseClient caches the reverse http client
func (d *Dialer) reverseClient() *http.Client {
	if d.revClient == nil {
		// create the http.client for the reverse connections
		tr := http.DefaultTransport.(*http.Transport)
		tr.DialContext = d.Dial
		client := http.Client{}
		client.Transport = tr
		d.revClient = &client
	}
	return d.revClient

}

// HTTP Handler that handles reverse connections and reverse proxy requests using 2 different paths:
// path base/revdial?key=id establish reverse connections and queue them so it can be consumed by the dialer
// path base/proxy/id/(path) proxies the (path) through the reverse connection identified by id
func (d *Dialer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// validate
	if r.Proto != "HTTP/2.0" {
		http.Error(w, "only HTTP/2.0 supported", http.StatusHTTPVersionNotSupported)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "expected GET request to conn handler", http.StatusMethodNotAllowed)
		return
	}

	// require TLS
	w.Header().Set("Strict-Transport-Security", "max-age=15768000 ; includeSubDomains")

	// process path
	path := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(path) == 0 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	// route the request
	pos := -1
	for i := len(path) - 1; i >= 0; i-- {
		p := path[i]
		// pathRevDial comes with a param
		if p == pathRevDial {
			if i != len(path)-1 {
				http.Error(w, "revdial: only last element on path allowed", http.StatusInternalServerError)
				return
			}
			pos = i
			break
		}
		// pathRevProxy requires at least the id subpath
		if p == pathRevProxy {
			if i == len(path)-1 {
				http.Error(w, "proxy: reverse path id required", http.StatusInternalServerError)
				return
			}
			pos = i
			break
		}
	}
	if pos < 0 {
		http.Error(w, "revdial: not handler ", http.StatusNotFound)
		return
	}
	// /base/proxy/id/..proxied path...
	if path[pos] == pathRevProxy {
		id := path[pos+1]
		newPath := "/"
		if len(path) > pos+1 {
			newPath = newPath + strings.Join(path[pos+2:], "/")
		}
		clone := r.Clone(context.TODO())
		clone.URL.Host = id
		clone.URL.Scheme = "http"
		clone.URL.Path = newPath
		clone.RequestURI = ""
		log.Printf("proxying request %v", clone)
		res, err := d.reverseClient().Do(clone)
		if err != nil {
			http.Error(w, "not reverse connection available", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()
		_, err = io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy server closed %v ", err)
	} else {
		// The caller identify itself by the value of the keu
		// https://server/revdial?id=dialerUniq
		dialerUniq := r.URL.Query().Get(urlParamKey)
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
		log.Printf("Connection from %s done", r.RemoteAddr)
	}
	return
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
