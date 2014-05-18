# gowebwatcher

A simple browser auto-reload utility for web developers.


### Build

gowebwatcher server is written in Go. Refer [here](http://golang.org/doc/install) for instructions on installing Go. Once you have a working Go environment:

```sh
$ go get github.com/porjo/gowebwatcher
$ go install github.com/porjo/gowebwatcher
```
You can then copy the resulting `gowebwatcher` binary (located in $GOPATH/bin) wherever you need it.

### Usage

* Start the `gowebwatcher` server in the directory that contains the files you want to watch for changes

```sh
  -ignores="": Ignored file pattens, seprated by ',', used to ignore the filesystem events of some files
  -port=8000: Which port to listen
  -private=false: Only listen on lookback interface, otherwise listen on all interface
  -root=".": Watched root directory for filesystem events, also the HTTP File Server's root directory
``` 

* Insert the following JS snippet into the web page(s) you want to auto-reload:
```Javascript
<script src="http://localhost:8000/js"></script> 
```

### Credits
Code based on [http-watcher](http://github.com/shenfeng/http-watcher)
