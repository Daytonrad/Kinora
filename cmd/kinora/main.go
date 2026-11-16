// Command kinora is a self-hosted media server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/scoty_lucas/kinora/internal/media"
	"github.com/scoty_lucas/kinora/internal/scanner"
	"github.com/scoty_lucas/kinora/internal/server"
	"github.com/scoty_lucas/kinora/internal/store"
)

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("kinora", server.Version)
		return
	}

	cfg := server.Config{}
	flag.StringVar(&cfg.Addr, "addr", env("KINORA_ADDR", ":8096"), "listen address (KINORA_ADDR)")
	flag.StringVar(&cfg.DataDir, "data", env("KINORA_DATA", "./data"), "data directory: database, images (KINORA_DATA)")
	flag.StringVar(&cfg.CacheDir, "cache", env("KINORA_CACHE", ""), "cache directory: transcodes, downloads, subtitles (KINORA_CACHE, default <data>/cache)")
	flag.StringVar(&cfg.WebDir, "web", env("KINORA_WEB_DIR", ""), "serve the web client from this directory instead of the embedded copy (KINORA_WEB_DIR)")
	flag.StringVar(&cfg.MediaRoot, "media-root", env("KINORA_MEDIA_ROOT", ""), "start folder of the admin folder picker (KINORA_MEDIA_ROOT)")
	flag.Parse()
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(cfg.DataDir, "cache")
	}

	log := newLogger()
	slog.SetDefault(log)
	if err := run(cfg, log); err != nil {
		log.Error("kinora stopped", "err", err)
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	lvl := slog.LevelInfo
	if strings.EqualFold(os.Getenv("KINORA_LOG_LEVEL"), "debug") {
		lvl = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(os.Getenv("KINORA_LOG_FORMAT"), "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func run(cfg server.Config, log *slog.Logger) error {
	ffmpeg := env("KINORA_FFMPEG", "ffmpeg")
	ffprobe := env("KINORA_FFPROBE", "ffprobe")
	for _, bin := range []string{ffmpeg, ffprobe} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%q nem található a PATH-on – telepítsd az ffmpeg csomagot (vagy add meg: KINORA_FFMPEG / KINORA_FFPROBE): %w", bin, err)
		}
	}
	for _, d := range []string{cfg.DataDir, cfg.CacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mappa létrehozása (%s): %w", d, err)
		}
	}

	st, err := store.Open(filepath.Join(cfg.DataDir, "kinora.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	for key, envName := range server.SettingEnv {
		if v := os.Getenv(envName); v != "" {
			st.SetOverride(key, v)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	media.FFmpeg, media.FFprobe = ffmpeg, ffprobe
	imgDir := filepath.Join(cfg.DataDir, "images")
	sc := scanner.New(st, imgDir, log)

	srv := server.New(cfg, st, log, server.Deps{Ctx: ctx, Scanner: sc, ImgDir: imgDir})
	go srv.Background(ctx)
	go sc.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("nem sikerült figyelni a(z) %s címen: %w", cfg.Addr, err)
	}
	log.Info("kinora started", "version", server.Version, "addr", cfg.Addr, "data", cfg.DataDir, "cache", cfg.CacheDir)

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.Serve(ln) }()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(shCtx)
	srv.Shutdown(shCtx)
	waitOrTimeout(shCtx, sc.Wait)
	log.Info("bye")
	return nil
}

// healthcheck is used by the Docker HEALTHCHECK (no curl needed in the image).
func healthcheck() int {
	addr := env("KINORA_ADDR", ":8096")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/api/status")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return 1
	}
	return 0
}

// waitOrTimeout runs f (a blocking wait) but gives up when ctx expires.
func waitOrTimeout(ctx context.Context, f func()) {
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
