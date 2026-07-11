package main

import (
	"flag"
	"github.com/gin-gonic/gin"
	"github.com/vibhordubey333/log-proxy/internal/proxy"
	"log"
	"net/http"
	"os"
)

func main() {
	jenkinsURL := flag.String("jenkins-url", "https://ci.jenkins.io", "base URL of the Jenkins server to proxy")
	cacheDir := flag.String("cache-dir", "./cache", "directory to store downloaded logs")
	addr := flag.String("addr", ":8080", "address to listen on")
	useFixture := flag.String("fixture", "", "path to a local log file to serve for all build IDs, instead of fetching from Jenkins")
	flag.Parse() // <-- must come after ALL flag.String(...) calls

	if err := os.MkdirAll(*cacheDir, 0755); err != nil {
		log.Fatalf("creating cache dir: %v", err)
	}

	var fetcher proxy.LogFetcher
	if *useFixture != "" {
		fetcher = &proxy.FixtureFetcher{FilePath: *useFixture, ContentType: "text/plain"}
		log.Printf("using fixture fetcher: serving %s for all build IDs", *useFixture)
	} else {
		fetcher = &proxy.JenkinsFetcher{BaseURL: *jenkinsURL, Client: http.DefaultClient}
	}

	cache := &proxy.Cache{Dir: *cacheDir, Fetcher: fetcher}
	handler := &proxy.Handler{Cache: cache}

	router := gin.Default()
	handler.RegisterRoutes(router)

	log.Printf("log proxy listening on %s, proxying %s, caching to %s", *addr, *jenkinsURL, *cacheDir)
	if err := router.Run(*addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
