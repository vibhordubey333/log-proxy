package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/vibhordubey333/log-proxy/internal/proxy"
)

func main() {
	jenkinsURL := flag.String("jenkins-url", "https://ci.jenkins.io", "base URL of the Jenkins server to proxy")
	cacheDir := flag.String("cache-dir", "./cache", "directory to store downloaded logs")
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	if err := os.MkdirAll(*cacheDir, 0755); err != nil {
		log.Fatalf("creating cache dir: %v", err)
	}

	cache := &proxy.Cache{
		Dir:     *cacheDir,
		Fetcher: &proxy.JenkinsFetcher{BaseURL: *jenkinsURL, Client: http.DefaultClient},
	}
	handler := &proxy.Handler{Cache: cache}

	router := gin.Default()
	handler.RegisterRoutes(router)

	log.Printf("log proxy listening on %s, proxying %s, caching to %s", *addr, *jenkinsURL, *cacheDir)
	if err := router.Run(*addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
