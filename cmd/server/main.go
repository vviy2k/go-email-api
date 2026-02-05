package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-email-api/internal/application"
	"go-email-api/internal/infrastructure/api"
	"go-email-api/internal/infrastructure/mail"
	"go-email-api/internal/infrastructure/storage"
)

func main() {
	addr := getEnv("APP_ADDR", ":8080")
	dbPath := getEnv("APP_DB_PATH", "./data/email.db")
	rawDir := getEnv("APP_RAW_DIR", "./data/raw")

	if err := os.MkdirAll("./data", 0o750); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		log.Fatalf("setup sqlite repository: %v", err)
	}
	defer repo.Close()
	store, err := storage.NewDiskRawEmailStore(rawDir)
	if err != nil {
		log.Fatalf("setup raw email store: %v", err)
	}

	service := application.NewService(repo, store, application.Clients{
		IMAP: mail.NewIMAPClient(),
		POP3: mail.NewPOP3Client(),
	})
	handler := api.NewHTTPHandler(service)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler.Routes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("email api listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
