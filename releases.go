package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

var downloadClient = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" || len(via) >= 10 {
		return errors.New("invalid download redirect")
	}
	return nil
}}

func fetchBytes(url string, limit int64) ([]byte, error) {
	res, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, errors.New("invalid download size")
	}
	return data, nil
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}
type releaseInfo struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

func latestRelease(repo string) (releaseInfo, error) {
	var release releaseInfo
	data, err := fetchBytes("https://api.github.com/repos/"+repo+"/releases/latest", 2<<20)
	if err != nil {
		return release, err
	}
	err = json.Unmarshal(data, &release)
	if err == nil && release.Tag == "" {
		err = errors.New("release has no version")
	}
	return release, err
}

func selectReleaseAsset(release releaseInfo, name string) (releaseAsset, error) {
	for _, asset := range release.Assets {
		match := asset.Name == name
		if name == "-arm64-mac.zip" {
			match = strings.HasSuffix(asset.Name, name)
		}
		if name == "-mac.zip" {
			match = strings.HasSuffix(asset.Name, name) && !strings.Contains(asset.Name, "arm64")
		}
		if match {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release %s has no %s asset", release.Tag, name)
}

func assetHash(release releaseInfo, asset releaseAsset) (string, error) {
	digest := strings.TrimPrefix(asset.Digest, "sha256:")
	if strings.HasPrefix(asset.Digest, "sha256:") && validHash(digest) {
		return strings.ToLower(digest), nil
	}
	for _, a := range release.Assets {
		if a.Name == "checksums.txt" {
			data, err := fetchBytes(a.URL, 1<<20)
			if err != nil {
				return "", err
			}
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset.Name && validHash(fields[0]) {
					return strings.ToLower(fields[0]), nil
				}
			}
		}
	}
	return "", errors.New("release has no SHA-256 for this asset; download stopped")
}

func validHash(hash string) bool {
	b, err := hex.DecodeString(hash)
	return err == nil && len(b) == sha256.Size
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}

// Cache by content hash. Interrupted or corrupted files are never executed.
func cachedAsset(asset releaseAsset, expected, cache string) (string, error) {
	if !validHash(expected) {
		return "", errors.New("invalid SHA-256")
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(cache, expected+filepath.Ext(asset.Name))
	if hash, err := hashFile(path); err == nil && hash == expected {
		return path, nil
	}
	f, err := os.CreateTemp(cache, ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	res, err := downloadClient.Get(asset.URL)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, (1<<30)+1))
	if err != nil {
		return "", err
	}
	if n > 1<<30 || hex.EncodeToString(h.Sum(nil)) != expected {
		return "", errors.New("SHA-256 mismatch; downloaded file was not installed")
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func releaseDownload(repo, name string, out io.Writer) (string, releaseInfo, error) {
	release, err := latestRelease(repo)
	if err != nil {
		return "", release, err
	}
	asset, err := selectReleaseAsset(release, name)
	if err != nil {
		return "", release, err
	}
	hash, err := assetHash(release, asset)
	if err != nil {
		return "", release, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", release, err
	}
	fmt.Fprintf(out, "Release %s / %s (SHA-256 verified; cached downloads reused)\n", release.Tag, asset.Name)
	path, err := cachedAsset(asset, hash, filepath.Join(cache, "wj", "downloads"))
	return path, release, err
}

func installOrca(plan installPlan, input io.Reader, out io.Writer) error {
	path, _, err := releaseDownload(plan.Repo, plan.Asset, out)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return runChild([]string{path, "/S"}, input, out, os.Environ())
	case "darwin":
		dir, err := os.MkdirTemp("", "setup-orca-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		if err = runChild([]string{"ditto", "-x", "-k", path, dir}, input, out, os.Environ()); err != nil {
			return err
		}
		return runChild([]string{"ditto", filepath.Join(dir, "Orca.app"), filepath.Join(home, "Applications", "Orca.app")}, input, out, os.Environ())
	case "linux":
		if err = replaceExecutable(path, filepath.Join(home, ".local", "bin", "orca-ide")); err != nil {
			return err
		}
		fmt.Fprintln(out, "Orca installed as ~/.local/bin/orca-ide. AppImage requires FUSE; launch with --appimage-extract-and-run if unavailable.")
	}
	return nil
}

func selfAsset(platform, arch string) string {
	if platform == "windows" && arch == "amd64" {
		return "wj.exe"
	}
	if platform == "darwin" {
		platform = "macos"
	}
	name := "wj-" + platform + "-" + arch
	if platform == "windows" {
		name += ".exe"
	}
	return name
}

func replaceExecutable(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := os.CreateTemp(filepath.Dir(dest), ".setup-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = io.Copy(f, in); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0o755); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	backup := ""
	if _, err = os.Stat(dest); err == nil {
		backup = dest + ".old-" + time.Now().UTC().Format("20060102T150405.000000000")
		if err = os.Rename(dest, backup); err != nil {
			return fmt.Errorf("cannot replace running executable; close other instances and use install.ps1: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = os.Rename(f.Name(), dest); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	// Windows may keep the running image locked until this process exits.
	if backup != "" {
		_ = os.Remove(backup)
	}
	removeOldExecutables(dest)
	return nil
}

// removeOldExecutables clears the backups earlier updates could not delete
// because Windows had the program open. The newest stays, as a way back.
func removeOldExecutables(dest string) {
	backups, _ := filepath.Glob(dest + ".old-*")
	sort.Strings(backups) // the name ends in a UTC time, so this is oldest first
	for i, backup := range backups {
		if i < len(backups)-1 {
			_ = os.Remove(backup)
		}
	}
}

func updateSelf(args []string, out io.Writer) error {
	if len(args) > 1 || len(args) == 1 && args[0] != "-dry-run" {
		return errors.New("usage: wj update [-dry-run]")
	}
	if len(args) == 1 {
		fmt.Fprintln(out, "Would fetch the latest wisjoycomm/wj release, verify SHA-256 and replace this executable.")
		return nil
	}
	path, release, err := releaseDownload("wisjoycomm/wj", selfAsset(runtime.GOOS, runtime.GOARCH), out)
	if err != nil {
		return err
	}
	dest, err := os.Executable()
	if err != nil {
		return err
	}
	dest, err = filepath.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	a, _ := hashFile(path)
	b, _ := hashFile(dest)
	if a == b {
		fmt.Fprintln(out, "Already up to date:", release.Tag)
		return nil
	}
	if err = replaceExecutable(path, dest); err != nil {
		return err
	}
	fmt.Fprintln(out, "Updated to", release.Tag, "- restart wj to use the new version.")
	return nil
}
