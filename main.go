package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func passwordFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	path := filepath.Join(home, ".zed-crypt")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot access ~/.zed-crypt: %w", err)
	}
	return path, nil
}

// ccryptDecrypt decrypts a .cpt file and returns the plaintext.
func ccryptDecrypt(path, keyfile string) ([]byte, error) {
	cmd := exec.Command("ccrypt", "-d", "-k", keyfile, "-c", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ccrypt decrypt failed: %w", err)
	}
	return out, nil
}

// ccryptEncrypt encrypts plaintext and writes to path as .cpt format.
func ccryptEncrypt(plaintext []byte, path, keyfile string) error {
	cmd := exec.Command("ccrypt", "-e", "-k", keyfile)
	cmd.Stdin = strings.NewReader(string(plaintext))
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ccrypt encrypt failed: %w", err)
	}
	return os.WriteFile(path, out, 0600)
}

func cmdEncrypt(path string) error {
	keyfile, err := passwordFile()
	if err != nil {
		return err
	}

	plaintext, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}

	outPath := path + ".cpt"
	if err := ccryptEncrypt(plaintext, outPath, keyfile); err != nil {
		return err
	}

	fmt.Printf("encrypted → %s\n", outPath)
	return nil
}

func cmdDecrypt(path string) error {
	keyfile, err := passwordFile()
	if err != nil {
		return err
	}

	plaintext, err := ccryptDecrypt(path, keyfile)
	if err != nil {
		return err
	}

	outPath := strings.TrimSuffix(path, ".cpt")
	if outPath == path {
		outPath = path + ".dec"
	}
	if err := os.WriteFile(outPath, plaintext, 0600); err != nil {
		return fmt.Errorf("cannot write %s: %w", outPath, err)
	}

	fmt.Printf("decrypted → %s\n", outPath)
	return nil
}

const daemonEnv = "ZED_CRYPT_DAEMON"

func cmdEdit(path string, foreground bool) error {
	// If not foreground and not already the daemon, re-exec as a detached background process
	if !foreground && os.Getenv(daemonEnv) == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot find own executable: %w", err)
		}
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Env = append(os.Environ(), daemonEnv+"=1")
		devNull, _ := os.Open(os.DevNull)
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		cmd.Stdin = devNull
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("cannot start background process: %w", err)
		}
		// Detach — don't wait for child
		cmd.Process.Release()
		return nil
	}

	keyfile, err := passwordFile()
	if err != nil {
		return err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Decrypt existing file, or start with empty content for new files
	var plaintext []byte
	if _, err := os.Stat(absPath); err == nil {
		plaintext, err = ccryptDecrypt(absPath, keyfile)
		if err != nil {
			return fmt.Errorf("decryption failed (wrong password?): %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", absPath, err)
	}

	// Determine a sensible extension for the temp file by stripping .cpt
	baseName := filepath.Base(absPath)
	tmpName := strings.TrimSuffix(baseName, ".cpt")
	// Use a stable hash so re-editing the same file reuses a predictable temp dir
	hash := sha256.Sum256([]byte(absPath))
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("zed-crypt-%x", hash[:8]))
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, tmpName)
	if err := os.WriteFile(tmpFile, plaintext, 0600); err != nil {
		return err
	}

	// Track content to detect changes
	lastContent := string(plaintext)

	// Re-encrypt helper
	reencrypt := func() error {
		current, err := os.ReadFile(tmpFile)
		if err != nil {
			return err
		}
		if string(current) == lastContent {
			return nil
		}
		if err := ccryptEncrypt(current, absPath, keyfile); err != nil {
			return err
		}
		lastContent = string(current)
		fmt.Printf("re-encrypted → %s\n", absPath)
		return nil
	}

	// Open in Zed (zed --wait blocks until the buffer is closed)
	cmd := exec.Command("zed", "--wait", tmpFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start zed: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	fmt.Printf("editing %s (watching for saves, ctrl-c or close buffer to finish)\n", absPath)

	for {
		select {
		case <-ticker.C:
			if err := reencrypt(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: re-encrypt failed: %v\n", err)
			}
		case err := <-done:
			if rerr := reencrypt(); rerr != nil {
				fmt.Fprintf(os.Stderr, "warning: final re-encrypt failed: %v\n", rerr)
			}
			return err
		case <-sig:
			if rerr := reencrypt(); rerr != nil {
				fmt.Fprintf(os.Stderr, "warning: final re-encrypt failed: %v\n", rerr)
			}
			cmd.Process.Kill()
			return nil
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `zed-crypt — transparent encryption for Zed editor

Usage:
  zed-crypt [--fg] <file.cpt>           decrypt, open in Zed, re-encrypt on save
  zed-crypt edit [--fg] <file.cpt>      same as above
  zed-crypt encrypt <file>              encrypt file → file.cpt
  zed-crypt decrypt <file.cpt>          decrypt file.cpt → file

Flags:
  --fg    run in foreground (default: background, no output)

Password is read from ~/.zed-crypt
Encryption: ccrypt format (Rijndael/AES, fully compatible with ccrypt CLI)
`)
	os.Exit(1)
}

// parseEditArgs extracts --fg flag and file path from args.
func parseEditArgs(args []string) (file string, fg bool) {
	for _, a := range args {
		if a == "--fg" {
			fg = true
		} else if file == "" {
			file = a
		}
	}
	return
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	var err error
	switch os.Args[1] {
	case "edit":
		file, fg := parseEditArgs(os.Args[2:])
		if file == "" {
			usage()
		}
		err = cmdEdit(file, fg)
	case "encrypt":
		if len(os.Args) < 3 {
			usage()
		}
		err = cmdEncrypt(os.Args[2])
	case "decrypt":
		if len(os.Args) < 3 {
			usage()
		}
		err = cmdDecrypt(os.Args[2])
	default:
		// Default command is edit
		file, fg := parseEditArgs(os.Args[1:])
		if file == "" {
			usage()
		}
		err = cmdEdit(file, fg)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
