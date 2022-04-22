package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aojea/h2rev2"
	"golang.org/x/net/http2"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

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

	// TODO pass a parameter to identify better the cluster
	if *id == "" {
		*id, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}

	//pathPrefix := ""
	target, _, err := rest.DefaultServerURL(config.Host, "", schema.GroupVersion{}, true)
	if err != nil {
		panic(err)
	}
	// target.Path = pathPrefix
	// target.Path = "/"
	transport, err := rest.TransportFor(config)
	if err != nil {
		panic(err)
	}
	fmt.Println("DEBUG target", target)

	upgradeTransport := proxy.NewUpgradeRequestRoundTripper(transport, proxy.MirrorRequest)
	proxy := proxy.NewUpgradeAwareHandler(target, transport, false, false, nil)
	proxy.UpgradeTransport = upgradeTransport
	proxy.UseRequestLocation = true
	proxy.UseLocationHost = true
	proxy.AppendLocationPath = false

	pathPrefix := "/proxy/" + *id

	proxRequestHandler := func(w http.ResponseWriter, r *http.Request) {
		req := r.Clone(r.Context())
		fmt.Println("DEBUG request before", req.URL)
		if len(pathPrefix) != 0 && pathPrefix != "/" {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, pathPrefix)
			if len(req.URL.RawPath) != 0 {
				req.URL.RawPath = strings.TrimPrefix(req.URL.EscapedPath(), pathPrefix)
			}
		}
		fmt.Println("DEBUG request after", req.URL)
		proxy.ServeHTTP(w, req)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", http.HandlerFunc(proxRequestHandler))

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

	l, err := h2rev2.NewListener(client, *dstURL, *id)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	log.Printf("Serving on Reverse connection")

	log.Fatal(http.Serve(l, mux))
}
