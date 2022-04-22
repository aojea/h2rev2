package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("v", "7")
	flag.Parse()

	target, err := url.Parse("https://192.168.1.141:9080")
	if err != nil {
		panic(err)
	}
	//target.Path = "/"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	upgradeTransport := proxy.NewUpgradeRequestRoundTripper(transport, proxy.MirrorRequest)
	proxy := proxy.NewUpgradeAwareHandler(target, transport, false, false, nil)
	proxy.UpgradeTransport = upgradeTransport
	proxy.UseRequestLocation = true
	proxy.UseLocationHost = true
	proxy.AppendLocationPath = false
	proxRequestHandler := func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)

	}
	cert, err := tls.X509KeyPair(sslipCert, sslipKey)
	if err != nil {
		log.Println(err)
		return
	}

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	ln, err := tls.Listen("tcp", ":8081", tlsConfig)
	if err != nil {
		log.Println(err)
		return
	}
	defer ln.Close()

	http.HandleFunc("/", proxRequestHandler)
	log.Fatal(http.Serve(ln, nil))

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
