// Command server runs the Not the End companion site: the static
// regolamento and the rules chatbot, in one self-contained binary.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Procionegobbo/nte-companion/content"
	"github.com/Procionegobbo/nte-companion/internal/chat"
	"github.com/Procionegobbo/nte-companion/internal/config"
	"github.com/Procionegobbo/nte-companion/internal/httpserver"
	"github.com/Procionegobbo/nte-companion/internal/ratelimit"
	"github.com/Procionegobbo/nte-companion/web"
)

func main() {
	// net/http's builtin mime table doesn't know .woff2 on every base image;
	// without this, some minimal container images serve fonts as
	// application/octet-stream, which several browsers refuse to load.
	_ = mime.AddExtensionType(".woff2", "font/woff2")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configurazione non valida: %v", err)
	}

	chapters, err := content.LoadAll(content.FS)
	if err != nil {
		log.Fatalf("caricamento regolamento fallito: %v", err)
	}

	systemPrompt := chat.Build(chapters)

	pages, err := httpserver.NewPageHandler(chapters, web.FS)
	if err != nil {
		log.Fatalf("inizializzazione pagine fallita: %v", err)
	}

	imgFS, err := fs.Sub(content.FS, "regolamento/img")
	if err != nil {
		log.Fatalf("sub-filesystem immagini fallito: %v", err)
	}
	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		log.Fatalf("sub-filesystem static fallito: %v", err)
	}

	client := chat.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.OpenRouterModel, cfg.MaxTokens, cfg.RequestTimeout)
	limiter := ratelimit.New(cfg.RateLimitPerMinute, cfg.RateLimitPerDay)
	defer limiter.Close()
	chatHandler := chat.NewHandler(client, systemPrompt, limiter)

	handler := httpserver.New(pages, chatHandler, imgFS, staticFS, cfg.TrustProxyHeaders)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
		// No WriteTimeout: the chat endpoint streams SSE for as long as the
		// model takes to answer. ReadHeaderTimeout alone is enough protection
		// against slow-header DoS on an otherwise unauthenticated endpoint.
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("nte-companion: avvio su :%s — %d capitoli caricati, modello %s, rate-limit %d/min %d/giorno",
		cfg.Port, len(chapters), cfg.OpenRouterModel, cfg.RateLimitPerMinute, cfg.RateLimitPerDay)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server interrotto: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("arresto in corso...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("arresto non pulito: %v", err)
	}
}
