package main

import (
	"io/fs"
	"log"

	"github.com/infopek/news-aggregator/internal/webassets"
)

func main() {
	if _, err := fs.ReadDir(webassets.Files, "dist"); err != nil {
		log.Fatal(err)
	}
}
