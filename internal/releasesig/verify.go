// Package releasesig verifies cosign detached signatures over release checksums.
package releasesig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
)

// Verifier parses a pinned PEM SubjectPublicKeyInfo ECDSA P-256 key. An absent
// production key fails closed; trust is never downloaded from the release host.
func Verifier(key []byte) (func([]byte, []byte) error, error) {
	block, rest := pem.Decode(key)
	if block == nil || block.Type != "PUBLIC KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("release public key is not provisioned or invalid")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	public, ok := parsed.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return nil, errors.New("release key must be ECDSA P-256")
	}
	return func(data, signature []byte) error {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		if !ecdsa.VerifyASN1(public, digest[:], raw) {
			return errors.New("invalid cosign signature")
		}
		return nil
	}, nil
}
