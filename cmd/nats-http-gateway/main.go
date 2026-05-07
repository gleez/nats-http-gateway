package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	natshttp "github.com/gleez/nats-http-gateway"
	"github.com/nats-io/nats.go"
)

var (
	urls      = flag.String("s", "", "The nats server URLs (separated by comma)")
	userCreds = flag.String("creds", "", "User Credentials File")
	nkeyFile  = flag.String("nkey", "", "NKey Seed File")
)

func main() {
	// Parse command‑line flags.
	flag.Parse()

	// Build NATS connection options from flags.
	opts := []nats.Option{nats.Name("nats-http-gateway"), nats.Timeout(5 * time.Second)}

	// Server URLs (default to NATS default).
	natsURL := *urls

	if *userCreds != "" && *nkeyFile != "" {
		log.Fatal("specify -seed or -creds")
	}

	// Credentials file (if provided).
	if *userCreds != "" {
		opts = append(opts, nats.UserCredentials(*userCreds))
	}

	// NKey seed file (if provided).
	if *nkeyFile != "" {
		if nkeyOpt, err := nats.NkeyOptionFromSeed(*nkeyFile); err == nil {
			opts = append(opts, nkeyOpt)
		} else {
			log.Fatalf("invalid NATS_SEED file: %v", err)
		}
	}

	// Connect to NATS.
	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Drain()

	// Initialize the HTTP handler.
	h := natshttp.New(nc)

	// Register the handler for all API paths.
	http.HandleFunc("/api/v1/", h.NatsHandler)

	// HTTP server configuration.
	srv := &http.Server{Addr: ":8080"}

	// Run server in a goroutine to allow graceful shutdown.
	go func() {
		log.Printf("starting HTTP server on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")
	if err := srv.Close(); err != nil {
		log.Printf("server close error: %v", err)
	}
	nc.Close()
	log.Println("exit")
}
