package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Khaymah/dhow/deploy/e2e-targets/notify-receiver/receiver"
)

const (
	defaultListen = "127.0.0.1:8443"
	headerTimeout = 5 * time.Second
	stopTimeout   = 5 * time.Second
	caFileMode    = 0o644
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(os.Args[1:], log); err != nil {
		log.Error("notify receiver stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("notify-receiver", flag.ContinueOnError)
	listen := fs.String("listen", defaultListen, "address to listen on")
	caFile := fs.String("ca-file", "", "file to write the PEM of the CA that signed the certificate (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *caFile == "" {
		return errors.New("-ca-file is required: the server under test must trust this CA")
	}
	cfg, caPEM, err := receiver.NewTLS(time.Now())
	if err != nil {
		return err
	}
	if err := os.WriteFile(*caFile, caPEM, caFileMode); err != nil {
		return err
	}
	srv := &http.Server{Addr: *listen, Handler: receiver.New().Handler(), TLSConfig: cfg, ReadHeaderTimeout: headerTimeout}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServeTLS("", "") }()
	log.Info("notify receiver listening", "address", *listen, "hosts", receiver.ServiceHosts())
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}
