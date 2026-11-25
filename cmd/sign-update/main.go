// Command sign-update emits a MyGo signature for a packaged update archive.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: sign-update PRIVATE_KEY_FILE PUBLIC_KEY_FILE ARCHIVE")
		os.Exit(2)
	}
	signature, err := sign(os.Args[1], os.Args[2], os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(signature)
}

func sign(privatePath, publicPath, archivePath string) (string, error) {
	info, err := os.Stat(privatePath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("the update private key must be a regular file accessible only by its owner")
	}
	private, err := readKey(privatePath, ed25519.PrivateKeySize)
	if err != nil {
		return "", err
	}
	defer clear(private)
	key := ed25519.NewKeyFromSeed(private[:ed25519.SeedSize])
	defer clear(key)
	public, err := readKey(publicPath, ed25519.PublicKeySize)
	if err != nil {
		return "", err
	}
	if !key.Equal(ed25519.PrivateKey(private)) || !key.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(public)) {
		return "", errors.New("the private key does not match the app's update public key")
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, archive); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest.Sum(nil))), nil
}

func readKey(path string, size int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	raw := bytes.TrimSpace(data)
	key := make([]byte, base64.StdEncoding.DecodedLen(len(raw)))
	n, err := base64.StdEncoding.Decode(key, raw)
	if err != nil || n != size {
		clear(key)
		return nil, fmt.Errorf("invalid MyGo key in %s", path)
	}
	return key[:n], nil
}
