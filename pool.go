// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go
package h2rev2

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

type controlMsg struct {
	Command  string `json:"command,omitempty"`  // "keep-alive", "conn-ready", "pickup-failed"
	ConnPath string `json:"connPath,omitempty"` // conn pick-up URL path for "conn-url", "pickup-failed"
	Err      string `json:"err,omitempty"`
}

type ReversePool struct {
	mu   sync.Mutex
	pool map[string]*Dialer
}

func NewReversePool() *ReversePool {
	return &ReversePool{
		pool: map[string]*Dialer{},
	}
}

// GetDialer returns a reverse dialer for the id
func (rp *ReversePool) Close() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	for _, v := range rp.pool {
		v.Close()
	}
}

// GetDialer returns a reverse dialer for the id
func (rp *ReversePool) GetDialer(id string) *Dialer {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.pool[id]
}

// CreateDialer creates a reverse dialer with id
// it's a noop if a dialer already exists
func (rp *ReversePool) CreateDialer(id string, conn net.Conn) *Dialer {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	if d, ok := rp.pool[id]; ok {
		return d
	}
	d := NewDialer(id, conn)
	rp.pool[id] = d
	return d

}

// HTTP Handler that handles reverse connections and reverse proxy requests using 2 different paths:
// path base/revdial?key=id establish reverse connections and queue them so it can be consumed by the dialer
// path base/proxy/id/(path) proxies the (path) through the reverse connection identified by id
func (rp *ReversePool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// recover panic
	defer func() {
		if r := recover(); r != nil {
			var err error
			switch t := r.(type) {
			case string:
				err = errors.New(t)
			case error:
				err = t
			default:
				err = errors.New("Unknown error")
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}()

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
	// Forward proxy /base/proxy/id/..proxied path...
	if path[pos] == pathRevProxy {
		id := path[pos+1]
		target, err := url.Parse("http://" + id)
		if err != nil {
			http.Error(w, "wrong url", http.StatusInternalServerError)
			return
		}
		d := rp.GetDialer(id)
		if d == nil {
			http.Error(w, "not reverse connections for this id available", http.StatusInternalServerError)
			return
		}
		transport := d.reverseClient().Transport
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Transport = transport
		proxy.Director = func(req *http.Request) {
			req.Host = target.Host
			originalDirector(req)
			klog.Infof("Forwarded request %s", req.URL)
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			klog.Infof("Forwarded response %d", resp.StatusCode)
			return nil
		}
		proxy.ServeHTTP(w, r)
		klog.V(5).Infof("proxy server closed %v ", err)
	} else {
		// The caller identify itself by the value of the keu
		// https://server/revdial?id=dialerUniq
		dialerUniq := r.URL.Query().Get(urlParamKey)
		if len(dialerUniq) == 0 {
			http.Error(w, "only reverse connections with id supported", http.StatusInternalServerError)
			return
		}

		d := rp.GetDialer(dialerUniq)
		// first connection to register the dialer and start the control loop
		if d == nil {
			conn := newConn(r.Body, flushWriter{w})
			d = rp.CreateDialer(dialerUniq, conn)
			// start control loop
			<-conn.Done()
			klog.V(5).Infof("stoped dialer %s control connection ", dialerUniq)
			return

		}
		// create a reverse connection
		klog.V(5).Infof("created reverse connection to %s %s id %s", r.RequestURI, r.RemoteAddr, dialerUniq)
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		conn := newConn(r.Body, flushWriter{w})
		select {
		case d.incomingConn <- conn:
		case <-d.Done():
			http.Error(w, "Reverse dialer closed", http.StatusInternalServerError)
			return
		}
		// keep the handler alive until the connection is closed
		<-conn.Done()
		klog.V(5).Infof("Connection from %s done", r.RemoteAddr)
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
