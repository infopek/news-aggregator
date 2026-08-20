package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: shutdown-fixture <url-file>")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[1], []byte("http://"+listener.Addr().String()+"/feed"), 0o600); err != nil {
		panic(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	if err := http.Serve(listener, handler); err != nil {
		panic(err)
	}
}
