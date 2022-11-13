module github.com/aojea/h2rev2/_examples/cmd/revclient

go 1.17

replace github.com/aojea/h2rev2 => ../../../

require (
	github.com/aojea/h2rev2 v0.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.0.0-20220411220226-7b82a4e95df4
	golang.org/x/sys v0.2.0
)

require (
	github.com/go-logr/logr v1.2.0 // indirect
	golang.org/x/net v0.2.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	k8s.io/klog/v2 v2.60.1 // indirect
)
