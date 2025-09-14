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
	baseDir    string
	listenAddr string
	tmpDirBase string

	l *slog.Logger
)

const (
	baseDirEnvKey    = "DNS_ZONE_RECEIVER_BASE_DIR"
	listenAddrEnvKey = "DNS_ZONE_RECEIVER_LISTEN_ADDR"
	tmpDirBaseEnvKey = "DNS_ZONE_RECEIVER_TMP_DIR"
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
	tmpDirBase = os.Getenv(tmpDirBaseEnvKey)
	if tmpDirBase == "" {
		tmpDirBase = "/tmp"
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

	if err = os.MkdirAll(tmpDirBase, 0755); err != nil {
		l.Error("failed to create temporary directory", slog.Any("error", err))
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	tmpFile, err := os.CreateTemp(tmpDirBase, "dns-zone-receiver-*")
	if err != nil {
		l.Error("failed to create temporary file", slog.Any("error", err))
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, zone); err != nil {
		l.Error("failed to save zone file", slog.Any("path", tmpFile.Name()), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}

	zonename := r.PathValue("zonename")
	outPath := filepath.Join(baseDir, zonename, "all.zone")
	err = os.MkdirAll(filepath.Dir(outPath), 0755)
	if err != nil {
		l.Error("failed to create output directory", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}

	out, err := os.Create(outPath)
	if err != nil {
		l.Error("failed to create output file", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	if err := os.Rename(tmpFile.Name(), outPath); err != nil {
		l.Error("failed to rename temporary file", slog.String("from", tmpFile.Name()), slog.String("to", outPath), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}

	l.Info("zone file uploaded successfully", slog.String("path", outPath))
	w.Write([]byte("done\n"))
}
