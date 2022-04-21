# How to run the server on fly.io


```sh
$ flyctl launch --image=$(ko build --sbom=none .)
```

Get the IP address:

```sh
$ flyctl ips list
TYPE ADDRESS           REGION CREATED AT 
v4   37.16.2.44        global 2m4s ago   
v6   2a09:8280:1::3267 global 2m4s ago   
```

check it is working:

```sh
$ curl -k https://37.16.2.44/revdial
only reverse connections with id supported

```



