module github.com/aojea/h2rev2/_examples/kubernetes/pod

go 1.17

require (
	github.com/aojea/h2rev2 v0.0.0-20220411092603-cfeb014aa4ff
	golang.org/x/sys v0.0.0-20220406163625-3f8b81556e12
	k8s.io/klog/v2 v2.60.1
)

replace github.com/aojea/h2rev2 => ../../../

require (
	github.com/go-logr/logr v1.2.0 // indirect
	golang.org/x/net v0.0.0-20220407224826-aac1ed45d8e3 // indirect
	golang.org/x/text v0.3.7 // indirect
)
