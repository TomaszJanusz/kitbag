package releasesig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestVerifier(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	verify, err := Verifier(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("fixture checksums")
	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	encoded := []byte(base64.StdEncoding.EncodeToString(sig))
	if err = verify(data, encoded); err != nil {
		t.Fatal(err)
	}
	if err = verify([]byte("changed"), encoded); err == nil {
		t.Fatal("accepted changed checksum file")
	}
	if err = verify(data, []byte("invalid base64!")); err == nil {
		t.Fatal("accepted invalid signature")
	}
}

func TestUnprovisionedKey(t *testing.T) {
	t.Parallel()
	for _, key := range [][]byte{nil, []byte("invalid"), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("bad DER")})} {
		if _, err := Verifier(key); err == nil {
			t.Fatal("accepted invalid key")
		}
	}
}
