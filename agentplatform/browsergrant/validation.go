package browsergrant

import (
	"encoding/base64"
	"errors"
	"regexp"
)

var protocolID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var digestID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var targetID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func unbase64(s string, n int) ([]byte, error) {
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil || len(b) != n || base64.RawURLEncoding.EncodeToString(b) != s {
		return nil, errors.New("noncanonical base64url")
	}
	return b, nil
}
