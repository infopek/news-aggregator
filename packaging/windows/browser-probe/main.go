package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := os.Getenv("NEWS_AGGREGATOR_BROWSER_PROBE_LOG")
	if path == "" {
		os.Exit(2)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(3)
	}
	defer file.Close()
	_, _ = fmt.Fprintln(file, strings.Join(os.Args[1:], " "))
}
