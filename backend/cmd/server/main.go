package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"footy/internal/api"
	"footy/internal/bus"
	"footy/internal/mail"
	"footy/internal/store"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Loopback by default: the demo is not reachable from the network unless
	// ADDR is changed on purpose.
	addr := env("ADDR", "127.0.0.1:8080")
	origins := strings.Split(env("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"), ",")

	var st store.Store = store.NewMemory()
	if uri := os.Getenv("MONGODB_URI"); uri != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		m, err := store.NewMongo(ctx, uri, env("MONGODB_DB", "footy"))
		cancel()
		if err != nil {
			// The URI may hold credentials; never log it.
			log.Error("mongodb connection failed", "err", err)
			os.Exit(1)
		}
		st = m
	}
	log.Info("storage ready", "kind", st.Name())
	// Testing aid only: GAME_SPEED=10 runs live series and lobbies ten times faster.
	if v, err := strconv.ParseFloat(os.Getenv("GAME_SPEED"), 64); err == nil && v > 1 {
		api.SetSpeed(v)
		log.Warn("live games accelerated", "speed", v)
	}

	// Mail: a real server when SMTP_HOST is set, otherwise printed to this log.
	var mailer mail.Mailer = mail.LogMailer{Log: log}
	if host := os.Getenv("SMTP_HOST"); host != "" {
		// SMTP_PASS is a secret: it is read here and never logged.
		mailer = mail.SMTP{Host: host, Port: env("SMTP_PORT", "587"), User: os.Getenv("SMTP_USER"), Pass: os.Getenv("SMTP_PASS"), From: env("MAIL_FROM", "no-reply@localhost")}
	}
	log.Info("mail ready", "kind", mailer.Name())
	publicURL := env("PUBLIC_URL", strings.TrimSpace(origins[0]))

	// Several instances share wake-ups and account rate limits through Redis.
	var b bus.Bus = bus.NewLocal()
	if url := os.Getenv("REDIS_URL"); url != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rb, err := bus.NewRedis(ctx, url)
		cancel()
		if err != nil {
			// The URL may hold a password; never log it.
			log.Error("redis connection failed", "err", err)
			os.Exit(1)
		}
		b = rb
	}
	defer b.Close()
	log.Info("bus ready", "kind", b.Name())

	game := api.New(st, api.Config{AllowedOrigins: origins, CookieSecure: os.Getenv("COOKIE_SECURE") == "true", Mailer: mailer, PublicURL: publicURL, FeedbackTo: feedbackTo(), Bus: b, TrustProxy: os.Getenv("TRUST_PROXY") == "true"}, log)
	janitor, stopJanitor := context.WithCancel(context.Background())
	defer stopJanitor()
	go game.Janitor(janitor)

	srv := &http.Server{
		Addr:              addr,
		Handler:           game.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	go func() {
		log.Info("listening", "addr", addr, "origins", origins)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = st.Close(ctx)
}

// feedbackTo is where in-game feedback is mailed: FEEDBACK_TO, or else the
// bare address of MAIL_FROM ("Kickoff Tactics <contact@...>" gives contact@...).
func feedbackTo() string {
	v := env("FEEDBACK_TO", os.Getenv("MAIL_FROM"))
	if a, err := netmail.ParseAddress(v); err == nil {
		return a.Address
	}
	return v
}
