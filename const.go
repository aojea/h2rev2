package h2rev2

// define the constants used to build the URL
// The listener connects to an user with path [host:port/base]/revdial?id=[id]
// The dialer listens on the urls:
// [host:port/base]/revdial for the reverse connections
// [host:port/base]/proxy/[id]/[path] for the reverse proxied to [path]
const (
	pathRevDial  = "revdial"
	pathRevProxy = "proxy"
	urlParamKey  = "id"
)
