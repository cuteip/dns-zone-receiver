package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuteip/dns-zone-receiver/internal/logger"
)

var (
	baseDir    string
	listenAddr string
	tmpDirBase string
	postHook   string

	l *slog.Logger
)

const (
	baseDirEnvKey    = "DNS_ZONE_RECEIVER_BASE_DIR"
	listenAddrEnvKey = "DNS_ZONE_RECEIVER_LISTEN_ADDR"
	tmpDirBaseEnvKey = "DNS_ZONE_RECEIVER_TMP_DIR"
	logLevelEnvKey   = "DNS_ZONE_RECEIVER_LOG_LEVEL"
	postHookEnvKey   = "DNS_ZONE_RECEIVER_POST_HOOK"
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
	postHook = os.Getenv(postHookEnvKey)

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

	// mktemp で group, other が落ちてしまうので上書き
	if err := os.Chmod(outPath, 0644); err != nil {
		l.Error("failed to change file permissions", slog.Any("path", outPath), slog.Any("error", err))
		http.Error(w, "failed to save zone file", http.StatusInternalServerError)
		return
	}

	if err := execPostHook(postHook, 10*time.Second, zonename); err != nil {
		l.Error("failed to execute post hook", slog.Any("error", err))
	}
	l.Info("zone file uploaded successfully", slog.String("path", outPath))
	w.Write([]byte("done\n"))
}

// ref: https://github.com/go-acme/lego/blob/0d567188f6fc2f2dac2f707bba888629ff92c6d4/cmd/hook.go#L26
func execPostHook(postHook string, timeout time.Duration, zonename string) error {
	if postHook == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	parts := strings.Fields(postHook)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("DNS_ZONE_RECEIVER_ZONENAME=%s", zonename))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	go func() {
		<-ctx.Done()
		if ctx.Err() != nil {
			_ = cmd.Process.Kill()
			_ = stdout.Close()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	err = cmd.Wait()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("hook timed out")
		}

		return fmt.Errorf("wait command: %w", err)
	}

	return nil
}
