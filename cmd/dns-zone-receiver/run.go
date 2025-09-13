package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cuteip/dns-zone-receiver/internal/logger"
)

var (
	baseDir    = ""
	listenAddr = ""

	l *slog.Logger
)

const (
	baseDirEnvKey    = "DNS_ZONE_RECEIVER_BASE_DIR"
	listenAddrEnvKey = "DNS_ZONE_RECEIVER_LISTEN_ADDR"
	logLevelEnvKey   = "DNS_ZONE_RECEIVER_LOG_LEVEL"
)

func main() {
	logLevel := os.Getenv(logLevelEnvKey)
	if logLevel == "" {
		logLevel = "info"
	}
	l = logger.SetupLogger(logLevel)

	if err := run(); err != nil {
		l.Error("Failed to execute command", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	baseDir = os.Getenv(baseDirEnvKey)
	if baseDir == "" {
		return fmt.Errorf("%s is not set", baseDirEnvKey)
	}
	listenAddr = os.Getenv(listenAddrEnvKey)
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8080"
	}

	http.HandleFunc("POST /v1/zones/{zonename}/upload", zoneUpload)
	l.Info("Listening on...\n", slog.String("address", listenAddr))
	return http.ListenAndServe(listenAddr, nil)
}

func zoneUpload(w http.ResponseWriter, r *http.Request) {
	zone, _, err := r.FormFile("zone")
	if err != nil {
		l.Error("failed to get 'zone' parameter", slog.Any("error", err))
		http.Error(w, "failed to get 'zone' parameter", http.StatusBadRequest)
		return
	}
	defer zone.Close()

	zonename := r.PathValue("zonename")
	outPath := filepath.Join(baseDir, zonename, "all.zone")
	err = os.MkdirAll(filepath.Dir(outPath), 0755)
	if err != nil {
		l.Error("failed to create output directory", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to create output directory", http.StatusInternalServerError)
		return
	}

	out, err := os.Create(outPath)
	if err != nil {
		l.Error("failed to create output file", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to create output file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, zone); err != nil {
		l.Error("failed to save zone file", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}

	l.Info("zone file uploaded successfully", slog.String("path", outPath))
	w.Write([]byte("done\n"))
}
