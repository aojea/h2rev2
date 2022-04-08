# h2rev2: Reverse proxy using http2 reverse connections

This package is based on https://pkg.go.dev/golang.org/x/build/revdial/v2 , however,
it uses HTTP2 to multiplex the reverse connections over HTTP2 streams, instead of using TCP connections.

This package implements a Dialer and Listener that works together to create reverse connections.

The motivation is that sometimes you want to run a server on a machine deep inside a NAT. 
Rather than connecting to the machine directly (which you can't, because of the NAT),
you have the sequestered machine connect out to a public machine.

The public machine runs the Dialer and the NAT machine the Listener.
The Listener opens a connections to the public machine that creates a reverse connection, so
the Dialer can use this connection to reach the NATed machine.

Typically, you would install a reverse proxy in the public machine and use the Dialer as a transport.

You can also install another reverse proxy in the NATed machine, to expose internal servers through this double reverse proxy chain.


## Example

1. Internal http server

```sh
python -m http.server
Serving HTTP on 0.0.0.0 port 8000 (http://0.0.0.0:8000/) ... 
```

2. 