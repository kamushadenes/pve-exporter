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

// parseVersion parses a version string into major, minor, patch integers
func parseVersion(version string) (major, minor, patch int) {
	version = strings.TrimPrefix(version, "v")
	parts := strings.Split(version, ".")
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &major)
	}
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &minor)
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &patch)
	}
	return
}

// compareVersions returns true if newVersion is newer than currentVersion
// Versions are expected in format "v1.2.3"
func compareVersions(currentVersion, newVersion string) bool {
	// Dev version is always considered older
	if currentVersion == "dev" {
		return true
	}

	curMajor, curMinor, curPatch := parseVersion(currentVersion)
	newMajor, newMinor, newPatch := parseVersion(newVersion)

	if newMajor != curMajor {
		return newMajor > curMajor
	}
	if newMinor != curMinor {
		return newMinor > curMinor
	}
	return newPatch > curPatch
}

// findAssetURL finds the download URL for the current platform
func findAssetURL(release *GitHubRelease) (string, error) {
	binaryName := getBinaryName()
	for _, asset := range release.Assets {
		if asset.Name == binaryName {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no binary found for platform %s/%s", runtime.GOOS, runtime.GOARCH)
}

// getExecutablePath returns the resolved path of the current executable
func getExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}
	return execPath, nil
}

// downloadBinary downloads a binary from URL to a temp file
func downloadBinary(downloadURL, execPath string) (string, error) {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "pve-exporter-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write binary: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to chmod binary: %w", err)
	}

	return tmpPath, nil
}

// verifyBinary checks if the downloaded binary is valid and executable
func verifyBinary(tmpPath string) error {
	cmd := exec.Command(tmpPath, "--version")
	output, err := cmd.CombinedOutput()
	if err == nil {
		fmt.Printf("New binary verified: %s", string(output))
		return nil
	}

	// Try with --help as fallback
	cmd = exec.Command(tmpPath, "--help")
	output, err = cmd.CombinedOutput()
	if err == nil {
		fmt.Printf("New binary verified: %s", string(output))
		return nil
	}

	// Check if file is executable using 'file' command
	cmd = exec.Command("file", tmpPath)
	fileOutput, _ := cmd.Output()
	if strings.Contains(string(fileOutput), "executable") {
		fmt.Println("New binary verified.")
		return nil
	}

	return fmt.Errorf("downloaded file is not a valid executable")
}

// replaceExecutable replaces the current executable with the new one
func replaceExecutable(execPath, tmpPath string) error {
	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		os.Rename(backupPath, execPath) // Try to restore backup
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	os.Remove(backupPath)
	return nil
}

// restartService attempts to restart the pve-exporter systemd service
func restartService() {
	fmt.Println("Restarting service...")
	cmd := exec.Command("systemctl", "restart", "pve-exporter")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: Failed to restart service: %v\n", err)
		fmt.Println("Please restart manually: systemctl restart pve-exporter")
		return
	}
	fmt.Println("Service restarted successfully!")
}

// SelfUpdate performs the self-update process
func SelfUpdate(currentVersion string) error {
	fmt.Println("Checking for updates...")

	release, err := CheckLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Latest version:  %s\n", release.TagName)

	if !compareVersions(currentVersion, release.TagName) {
		fmt.Println("Already running the latest version!")
		return nil
	}

	downloadURL, err := findAssetURL(release)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", getBinaryName())

	execPath, err := getExecutablePath()
	if err != nil {
		return err
	}

	tmpPath, err := downloadBinary(downloadURL, execPath)
	if err != nil {
		return err
	}

	if err := verifyBinary(tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := replaceExecutable(execPath, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	fmt.Println("Update successful!")
	restartService()
	return nil
}
