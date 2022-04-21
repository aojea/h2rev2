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
	"time"

	"github.com/aojea/h2rev2"
	"golang.org/x/net/http2"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/proxy"
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

	pathPrefix := "/proxy/" + *id + "/"

	server, err := proxy.NewServer("", pathPrefix, "", nil, config, 30*time.Second, false)
	if err != nil {
		panic(err)
	}

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
	log.Fatal(server.ServeOnListener(l))
}
