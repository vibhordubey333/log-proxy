package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vibhordubey333/log-proxy/internal/client"
	"github.com/vibhordubey333/log-proxy/internal/dedup"
)

func main() {
	proxyURL := flag.String("proxy", "http://localhost:8080", "base URL of the log proxy")
	buildID := flag.String("build", "", "build ID to fetch (required)")
	flag.Parse()

	if *buildID == "" {
		fmt.Fprintln(os.Stderr, "error: --build is required")
		flag.Usage()
		os.Exit(1)
	}

	lines, err := client.FetchLog(*proxyURL, *buildID)
	if err != nil {
		log.Fatalf("fetching log: %v", err)
	}

	lines = dedup.StripTimestamps(lines)
	lines = dedup.CollapseConsecutive(lines)

	for _, line := range lines {
		fmt.Println(line)
	}
}
