package browsergrant

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestSharedSigningAndRasterVectors(t *testing.T) {
	b, e := os.ReadFile("testdata/signing-vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Records []struct {
			Domain                                  string
			Record                                  map[string]any
			Canonical, Digest, PublicKey, Signature string
		}
		Raster struct{ Base64, Digest string }
	}
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	for _, r := range v.Records {
		bytes, e := SignedBytes(r.Domain, r.Record)
		if e != nil {
			t.Fatal(e)
		}
		digest, e := Digest(r.Domain, r.Record)
		if e != nil || digest != r.Digest {
			t.Fatal("digest mismatch", e)
		}
		pub, _ := base64.RawURLEncoding.DecodeString(r.PublicKey)
		sig, _ := base64.RawURLEncoding.DecodeString(r.Signature)
		if !ed25519.Verify(pub, bytes, sig) {
			t.Fatal("cross-language signature mismatch")
		}
	}
	png, _ := base64.StdEncoding.DecodeString(v.Raster.Base64)
	sum := sha256.Sum256(append([]byte("iugum/browser/raster/2\n"), png...))
	if hex.EncodeToString(sum[:]) != v.Raster.Digest {
		t.Fatal("raster digest mismatch")
	}
}
