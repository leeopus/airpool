package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/airpool/airpool/internal/alert"
	"github.com/airpool/airpool/internal/api"
	"github.com/airpool/airpool/internal/checker"
	"github.com/airpool/airpool/internal/config"
	"github.com/airpool/airpool/internal/db"
	"github.com/airpool/airpool/internal/store"
	"github.com/airpool/airpool/internal/subscribe"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "airpool.toml", "config file path")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Println("airpool-server", version)
		os.Exit(0)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("airpool-server %s starting...", version)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Open database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// Init store
	s := store.New(database)

	// Init alerter
	alerter := alert.New(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.AlertEmail)
	if alerter.Enabled() {
		log.Println("alerting enabled")
	}

	// Init subscription generator
	gen := subscribe.New(s, cfg.Hysteria2Password)

	// Init health checker
	chk := checker.New(s, alerter, cfg.CheckInterval, cfg.CheckTimeout, cfg.OfflineThreshold)
	chk.Start()
	defer chk.Stop()

	// Init API handler
	handler := api.New(cfg, s, gen)
	mux := http.NewServeMux()
	handler.Register(mux)

	// Print subscribe URL
	log.Printf("Subscribe URL: http://<IP>%s/api/subscribe?token=%s", cfg.Listen, cfg.SubscribeToken)

	// Start HTTP server
	server := &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
	}

	go func() {
		log.Printf("listening on %s", cfg.Listen)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Wait for signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
}
