package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/aojea/h2rev2"
	"golang.org/x/sys/unix"
)

var (
	flagPort     string
	flagCert     string
	flagKey      string
	flagBasePath string
)

func init() {
	flag.StringVar(&flagPort, "port", "8080", "Specify the default port to listen")
	flag.StringVar(&flagCert, "cert", "", "Specify the server certificate file")
	flag.StringVar(&flagKey, "key", "", "Specify the server certificate key file")
	flag.StringVar(&flagBasePath, "base-path", "/", "Specify the base-path the reverse dialer handler should use")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: h2rev2server [options]\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	// Parse command line flags and arguments
	flag.Parse()

	// trap Ctrl+C and call cancel on the context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	// Enable signal handler
	signalCh := make(chan os.Signal, 2)
	defer func() {
		close(signalCh)
		cancel()
	}()

	signal.Notify(signalCh, os.Interrupt, unix.SIGINT)
	go func() {
		select {
		case <-signalCh:
			log.Printf("Exiting: received signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	dialer := h2rev2.NewDialer()
	defer dialer.Close()

	mux := http.NewServeMux()
	mux.Handle(flagBasePath, dialer)

	// Create a server on port 8000
	// Exactly how you would run an HTTP/1.1 server
	srv := &http.Server{
		Addr:    "0.0.0.0:" + flagPort,
		Handler: mux,
	}
	defer srv.Close()

	log.Printf("Serving on %s", srv.Addr)
	errCh := make(chan error)
	go func() {
		if flagCert != "" && flagKey != "" {
			errCh <- srv.ListenAndServeTLS(flagCert, flagKey)
		} else {
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(sslipCert) {
				errCh <- fmt.Errorf("Failed to append Cert from PEM")
			}
			cert, err := tls.X509KeyPair(sslipCert, sslipKey)
			if err != nil {
				errCh <- err
			}
			srv.TLSConfig = &tls.Config{
				RootCAs:      roots,
				Certificates: []tls.Certificate{cert},
			}

			errCh <- srv.ListenAndServeTLS("", "")
		}
	}()
	var err error
	select {
	case err = <-errCh:
	case <-ctx.Done():
		err = srv.Close()
	}
	if err != nil {
		log.Printf("Exiting with error: %v", err)
		os.Exit(1)
	}
}

// sslipCert was generated from crypto/tls/generate_cert.go with the following command:
//     go run generate_cert.go  --rsa-bits 2048 --host 127.0.0.1,::1,sslip.io --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var sslipCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDNjCCAh6gAwIBAgIQbdYY7YxjrPohiTK9EwZW4DANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA5BcFh907JD/G9iGCLaECM2vUSONGDbMRl/ReBIYrRLe6B6esnPoA
uBp5xa4rpIDUKwQYKJApaZ20aHveaCWcrAoJ7RogUltdHIhMfv+BvSEf0qP+JVUT
RtFFoikvsCGm/fDQ1TqCqFBUAsssqbPlpVcrIUpBYDbOjZ/fk+PtD5NIK8WLYFsa
3E31riAJbOzsAZC2X9xFHKYYnlTHFcoeJMn9OzJe6uzF9sjv1iOIOMjHhaFYWo8T
dM8G1Widzk1waDhAcys2wxbkUQoy4tmAWoQbOB26lX0TUjHhR5Bt1Dr/xevAGG28
RjZquRvVplSPHJGfMxivVvu/Pc7Pav44lwIDAQABo4GFMIGCMA4GA1UdDwEB/wQE
AwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1Ud
DgQWBBR744nTcf+iQNU5D6MUVMEBuXC83jArBgNVHREEJDAigghzc2xpcC5pb4cE
fwAAAYcQAAAAAAAAAAAAAAAAAAAAATANBgkqhkiG9w0BAQsFAAOCAQEAFpVZTFWj
onDbCGqDqLpcjhEVqwsjvQHwHuTZ/PfC+pbpVWbja0ORx/XzWJbbig04n3zwzQkE
G8/0Jqg4FU/nae1McdkH0EKZ8hwFddGyKH4DDJs1w6A/6/r9bXOcda2Drv0abJgR
gPUZUpFwSA53KZprxev6GfUbNaG2N+3K4r/G1JUffkWeAAqkBh06wdyQakkXOwOe
1eUcSlMrvhmZm99jzCaiOoRRpU/TpysSNdEKL1A+biCUtAu9/a33ZiEZMEF8E4/D
uXxE3Krh6t7SgBjHgjaWOqUBs0ZO6VPKkTcQnSui9AoVPxiksnK6PDC8Z+tAhzZI
YNkQ9fr4zT30Qw==
-----END CERTIFICATE-----`)

// sslipKey is the private key for localhostCert.
var sslipKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQDkFwWH3TskP8b2
IYItoQIza9RI40YNsxGX9F4EhitEt7oHp6yc+gC4GnnFriukgNQrBBgokClpnbRo
e95oJZysCgntGiBSW10ciEx+/4G9IR/So/4lVRNG0UWiKS+wIab98NDVOoKoUFQC
yyyps+WlVyshSkFgNs6Nn9+T4+0Pk0grxYtgWxrcTfWuIAls7OwBkLZf3EUcphie
VMcVyh4kyf07Ml7q7MX2yO/WI4g4yMeFoVhajxN0zwbVaJ3OTXBoOEBzKzbDFuRR
CjLi2YBahBs4HbqVfRNSMeFHkG3UOv/F68AYbbxGNmq5G9WmVI8ckZ8zGK9W+789
zs9q/jiXAgMBAAECggEBALlBDYftIrTta/7K9n1y8WOsZ84Pcf18fISrwJTyGECG
7Px8rlENKPpe3pq1PNMuo6SQfcKsXEZhBX97ZAe4zMhamvdNqgTaGgUrmt3nTou7
VKpz8d6Ge9Kf9GuiAg6PNp+4MRWOoUJtg96FALCQ4atp4ij2s6SevyL+P8xRamCj
mYf4BjJFYHM+jMbfnQJ/a2YT6/sxqbjx4n+oRjnvZFaqhSjBLBrYjS58UDQ5DMBt
tfNpEGp/vTD/5BXgpEX54ZYsdTRYDcv2bxEUim89d4EqcMijpkkAzTnFIX1KUSoJ
3hK4dMSTaPR97I/VbqPUe0jy5o/HnEDx19QtIBTUFqECgYEA8APfFak1sv7xGF8+
XvYfLpAn2yR1ZwRPD0TSKQ0Z77DK6ANFwejnSX2Csf4AvXTRTranw2M5ykCZPZTo
+25BjHvGAbENnZ/0T42EP3ws/HeqHtfv4J63VUJSXBPjTX6l8uHeq6o5HOSDPHOT
GqaqU4RDx4dlNktJQmpSw/qF4DECgYEA80fVADmWxQZWwuDehWW+HV+22G/tVY7P
4WF/7ymv+SyYvBWZEdtxJ4F3nJUNlSrKJDNnqRTJBCgSHUX/PdiQjh5jD14H1IOd
kmyF3xFk2LsNzXqQ9ZJWTq2cQm5KGmv9NkSlQH+67XVCp550jX7gx9IfNANMfl32
MNPnN1+d+0cCgYEAgidnSyTGRPmxHilP9kj7YdG0e0bLD4ErqjkEylQbc3poneZg
ZqX4/kY8oG8AUbzOYCP216KwTPg44UcmDGqeyyK3nmU33/lEj/tK8u5QqtvteepZ
X3JSMr7TULFMOtLqBMrtaCPX8s4MSLTX2cT1anK4GrRWc1niMUzc8v+gp5ECgYB9
t9InsprqKBNv05rKXsB3F346rOR9wTZF5wegxO8uGdC36YVXiAoaezofjZseSaV6
PaJE6vvSDQ8HV6PGBwL0nllcmJ/9PyKPh0tK8gcmRMumMr90V/IH6ImGfs4Gh2Wr
xJ+NDDTB/0W5rxXWBQoN2NTNISNHbjEKHIcww1W1gwKBgQCWmsjK9f1eRO1wOcCd
L3EsJzGvdEUJtMeuMQb56kqAGPsEaQ87Pdc3xn9GsfxaDMlJhqMQE9g1Hfs0JL/M
SX4+W3Tr4WOi/wpc7PUB/b5JIBO6ct/Z0nWVxyY5wDmrMN4eOzZaBmPshRjkUZ3R
0ky9kyVw1o2DT4b2F86HyjKD9Q==
-----END PRIVATE KEY-----`)
