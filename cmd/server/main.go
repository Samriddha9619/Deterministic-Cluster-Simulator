package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Samriddha9619/deterministic-cluster-simulator/server"
)

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	staticDir := flag.String("static", "./web", "Directory containing static frontend files")
	flag.Parse()

	absStatic, err := filepath.Abs(*staticDir)
	if err != nil {
		absStatic = *staticDir
	}

	if _, err := os.Stat(absStatic); os.IsNotExist(err) {
		log.Printf("Warning: static directory %s does not exist yet", absStatic)
	}

	srv := server.NewServer(server.Config{
		Port:      *port,
		StaticDir: absStatic,
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Deterministic Cluster Simulator UI running at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
