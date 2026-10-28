// Package browsergrant implements the optional signed browser protocol. Server
// routes require explicit --browser-grants and --host-tools; default is disabled.
// This candidate is not deployed and establishes no actual browser capability.
package browsergrant

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Canonical encodes the ASCII-key/string subset of RFC8785 used by the typed
// browser records. Rejecting Unicode here is deliberate until an audited full
// UTF16 sorter/string parser exists; callers cannot silently use Go JSON bytes.
func Canonical(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		if x {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case string:
		for _, c := range x {
			if c < 32 || c > 126 {
				return nil, errors.New("non-ASCII protocol string")
			}
		}
		var out bytes.Buffer
		enc := json.NewEncoder(&out)
		enc.SetEscapeHTML(false)
		if e := enc.Encode(x); e != nil {
			return nil, e
		}
		return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
	case int:
		return Canonical(float64(x))
	case int64:
		if x > 9007199254740991 || x < -9007199254740991 {
			return nil, errors.New("unsafe integer")
		}
		return Canonical(float64(x))
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x == 0 && math.Signbit(x) || math.Trunc(x) == x && math.Abs(x) > 9007199254740991 {
			return nil, errors.New("invalid number")
		}
		if x == 0 {
			return []byte("0"), nil
		}
		format := byte('f')
		if math.Abs(x) < 1e-6 || math.Abs(x) >= 1e21 {
			format = 'e'
		}
		s := strconv.FormatFloat(x, format, -1, 64)
		if i := strings.IndexByte(s, 'e'); i >= 0 {
			mant, exp := s[:i], s[i+1:]
			n, _ := strconv.Atoi(exp)
			sign := ""
			if n >= 0 {
				sign = "+"
			}
			s = mant + "e" + sign + strconv.Itoa(n)
		}
		return []byte(s), nil
	case []any:
		parts := make([]string, len(x))
		for i, v := range x {
			b, e := Canonical(v)
			if e != nil {
				return nil, e
			}
			parts[i] = string(b)
		}
		return []byte("[" + strings.Join(parts, ",") + "]"), nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			kb, e := Canonical(k)
			if e != nil {
				return nil, e
			}
			b, e := Canonical(x[k])
			if e != nil {
				return nil, e
			}
			parts = append(parts, string(kb)+":"+string(b))
		}
		return []byte("{" + strings.Join(parts, ",") + "}"), nil
	default:
		return nil, errors.New("untyped protocol value")
	}
}
func SignedBytes(domain string, record map[string]any) ([]byte, error) {
	b, e := Canonical(record)
	if e != nil {
		return nil, e
	}
	return append([]byte("iugum/browser/"+domain+"/2\n"), b...), nil
}
func Digest(domain string, v any) (string, error) {
	b, e := Canonical(v)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(append([]byte("iugum/browser/"+domain+"/2\n"), b...))
	return hex.EncodeToString(sum[:]), nil
}
