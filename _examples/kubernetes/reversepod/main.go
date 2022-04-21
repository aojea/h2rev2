package main

import (
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
	"path/filepath"

	"github.com/aojea/h2rev2"
	"golang.org/x/net/http2"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

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
		log.Printf("rev proxy rvc request %+v", r)
		proxy.ServeHTTP(w, r)
	}
}

func main() {
	klog.InitFlags(nil)

	// syncer -----> apiserver
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	dstURL := flag.String("url", "", "server url")
	certFile := flag.String("certFile", "", "server certificate file")
	id := flag.String("id", "", "identifies (default: hostname)")
	flag.Set("v", "7")
	flag.Parse()

	// validation
	var config *rest.Config
	var err error
	if *kubeconfig != "" {
		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	if *dstURL == "" {
		panic(fmt.Errorf("empty url"))
	}

	// initialize a reverse proxy and pass the actual backend server url here
	proxy, err := NewProxy(config.Host)
	if err != nil {
		panic(err)
	}
	transport, err := rest.TransportFor(config)
	if err != nil {
		panic(err)
	}
	proxy.Transport = transport

	// kcp  -----> syncer (reverse connection)
	client := &http.Client{}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	if *certFile != "" {
		caCert, err := ioutil.ReadFile(*certFile)
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
	// TODO pass a parameter to identify better the cluster
	if *id == "" {
		*id, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}
	l, err := h2rev2.NewListener(client, *dstURL, *id)
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
	log.Fatal(server.Serve(l))
}
