package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	repoOwner  = "bigtcze"
	repoName   = "pve-exporter"
	releaseAPI = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

// GitHubRelease represents the GitHub release API response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckLatestVersion queries GitHub API for the latest release
func CheckLatestVersion() (*GitHubRelease, error) {
	resp, err := http.Get(releaseAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	return &release, nil
}

// getBinaryName returns the expected binary name for the current platform
func getBinaryName() string {
	return fmt.Sprintf("pve-exporter-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// compareVersions returns true if newVersion is newer than currentVersion
// Versions are expected in format "v1.2.3"
func compareVersions(currentVersion, newVersion string) bool {
	// Dev version is always considered older
	if currentVersion == "dev" {
		return true
	}

	// Strip 'v' prefix if present
	current := strings.TrimPrefix(currentVersion, "v")
	new := strings.TrimPrefix(newVersion, "v")

	// Simple string comparison works for semantic versioning
	// For more robust comparison, use a semver library
	return new > current
}

// SelfUpdate performs the self-update process
func SelfUpdate(currentVersion string) error {
	fmt.Println("Checking for updates...")

	// Get latest release info
	release, err := CheckLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	// Compare versions
	if !compareVersions(currentVersion, release.TagName) {
		fmt.Println("Already running the latest version!")
		return nil
	}

	// Find the correct binary asset
	binaryName := getBinaryName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	fmt.Printf("Downloading %s...\n", binaryName)

	// Get the current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Download new binary to temp file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file in the same directory (for atomic rename)
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "pve-exporter-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Copy downloaded content
	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to chmod binary: %w", err)
	}

	// Verify the new binary is executable and runs
	// Try --version first, fall back to --help, then just check if it's executable
	cmd := exec.Command(tmpPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with --help as fallback (for older versions without --version)
		cmd = exec.Command(tmpPath, "--help")
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Just check if the binary can start (it may exit quickly without args)
			cmd = exec.Command("file", tmpPath)
			fileOutput, _ := cmd.Output()
			if !strings.Contains(string(fileOutput), "executable") {
				os.Remove(tmpPath)
				return fmt.Errorf("downloaded file is not a valid executable")
			}
		}
	}
	if len(output) > 0 {
		fmt.Printf("New binary verified: %s", string(output))
	} else {
		fmt.Println("New binary verified.")
	}

	// Backup current binary
	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary to target path
	if err := os.Rename(tmpPath, execPath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	fmt.Println("Update successful!")
	fmt.Println("Restarting service...")

	// Restart via systemctl
	cmd = exec.Command("systemctl", "restart", "pve-exporter")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: Failed to restart service: %v\n", err)
		fmt.Println("Please restart manually: systemctl restart pve-exporter")
		return nil
	}

	fmt.Println("Service restarted successfully!")
	return nil
}
