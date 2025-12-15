package core

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WinTunVersion - версия wintun.dll для скачивания
const WinTunVersion = "0.14.1"

// WinTunDownloadURL - URL для скачивания wintun.dll
const WinTunDownloadURL = "https://www.wintun.net/builds/wintun-%s.zip"

// CheckWintunDLL проверяет наличие wintun.dll
func (ac *AppController) CheckWintunDLL() (bool, error) {
	if runtime.GOOS != "windows" {
		return true, nil // На не-Windows системах wintun не нужен
	}

	if _, err := os.Stat(ac.WintunPath); os.IsNotExist(err) {
		return false, nil
	}
	return true, nil
}

// DownloadWintunDLL downloads and installs wintun.dll
func (ac *AppController) DownloadWintunDLL(ctx context.Context, progressChan chan DownloadProgress) {
	defer close(progressChan)

	if runtime.GOOS != "windows" {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  "wintun.dll is only needed on Windows",
			Status:   "error",
			Error:    fmt.Errorf("wintun.dll is only needed on Windows"),
		}
		return
	}

	// 1. Создаем временную директорию
	tempDir := filepath.Join(ac.ExecDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create temp dir: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// 2. Скачиваем ZIP архив
	zipURL := fmt.Sprintf(WinTunDownloadURL, WinTunVersion)
	zipPath := filepath.Join(tempDir, fmt.Sprintf("wintun-%s.zip", WinTunVersion))

	progressChan <- DownloadProgress{Progress: 10, Message: "Downloading wintun.dll...", Status: "downloading"}
	if err := ac.downloadFileFromURL(ctx, zipURL, zipPath, progressChan); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Download failed: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}

	// 3. Распаковываем ZIP и извлекаем wintun.dll
	progressChan <- DownloadProgress{Progress: 80, Message: "Extracting wintun.dll...", Status: "extracting"}

	// Определяем архитектуру
	var archDir string
	switch runtime.GOARCH {
	case "amd64":
		archDir = "amd64"
	case "arm64":
		archDir = "arm64"
	default:
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Unsupported architecture: %s", runtime.GOARCH),
			Status:   "error",
			Error:    fmt.Errorf("unsupported architecture: %s", runtime.GOARCH),
		}
		return
	}

	// Открываем ZIP
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to open zip: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}
	defer func() { _ = r.Close() }()

	// Ищем wintun.dll в нужной директории
	var dllPath string
	targetPath := fmt.Sprintf("bin/%s/wintun.dll", archDir)

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, targetPath) || strings.Contains(f.Name, fmt.Sprintf("/%s/wintun.dll", archDir)) {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			// Извлекаем файл
			dllPath = filepath.Join(tempDir, "wintun.dll")
			outFile, err := os.Create(dllPath)
			if err != nil {
				_ = rc.Close()
				continue
			}

			_, err = io.Copy(outFile, rc)
			_ = outFile.Close()
			_ = rc.Close()

			if err != nil {
				continue
			}

			break
		}
	}

	if dllPath == "" {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  "wintun.dll not found in archive",
			Status:   "error",
			Error:    fmt.Errorf("wintun.dll not found in archive"),
		}
		return
	}

	// 4. Копируем wintun.dll в целевую директорию
	progressChan <- DownloadProgress{Progress: 90, Message: "Installing wintun.dll...", Status: "extracting"}

	// Создаем директорию bin если её нет
	binDir := filepath.Dir(ac.WintunPath)
	if err := os.MkdirAll(binDir, 0755); err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create bin directory: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}

	// Копируем файл
	sourceFile, err := os.Open(dllPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to open source file: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(ac.WintunPath)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to create destination file: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		progressChan <- DownloadProgress{
			Progress: 0,
			Message:  fmt.Sprintf("Failed to copy file: %v", err),
			Status:   "error",
			Error:    err,
		}
		return
	}

	// 5. Готово!
	progressChan <- DownloadProgress{
		Progress: 100,
		Message:  fmt.Sprintf("wintun.dll v%s installed successfully!", WinTunVersion),
		Status:   "done",
	}
}
