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

2. Internal h2rev2 client: it will create a reverse connection and proxy the internal server

```sh
$ ./main -cert /tmp/server.crt -dialer-key dialerid -dialer-id 001 -dialer-path /revdial -proxy-host http://localhost:8000 localhost 9090
2022/04/08 13:31:12 Reversing proxy to http://localhost:8000
2022/04/08 13:31:12 Serving on Reverse connection
2022/04/08 13:31:12 Listener creating connection to https://localhost:9090/revdial?dialerid=001

```

3. Public h2rev2 server: it will expose the internal server through the reverse connection created by the internal h2rev2 client

```sh
$ ./main -public -cert /tmp/server.crt -key /tmp/server.key -dialer-key dialerid -dialer-id 001 -dialer-path /revdial localhost 9090
2022/04/08 13:31:09 Serving on localhost:9090



2022/04/08 13:31:12 created reverse connection to /revdial?dialerid=001 127.0.0.1:36742 id 001
2022/04/08 13:31:15 Dialing tcp 001:80
```

4. Connect to the public h2rev2 server and it will forward the request to the internal server

```sh
$ curl -k https://localhost:9090/get
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN"
        "http://www.w3.org/TR/html4/strict.dtd">
<html>
```
