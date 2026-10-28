package browsergrant

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
)

// Decode rejects duplicate and unknown numeric representations before typed
// protocol dispatch. Schema-specific unknown-key validation remains mandatory.
func Decode(r io.Reader) (map[string]any, error) {
	d := json.NewDecoder(r)
	d.UseNumber()
	v, e := readValue(d, "")
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("trailing protocol data")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("protocol object required")
	}
	return m, nil
}
func readValue(d *json.Decoder, key string) (any, error) {
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch x := t.(type) {
	case json.Delim:
		if x == '{' {
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				s, ok := k.(string)
				if !ok {
					return nil, errors.New("object key required")
				}
				if _, ok = m[s]; ok {
					return nil, errors.New("duplicate protocol key")
				}
				v, e := readValue(d, s)
				if e != nil {
					return nil, e
				}
				m[s] = v
			}
			_, e = d.Token()
			return m, e
		}
		if x == '[' {
			var a []any
			for d.More() {
				v, e := readValue(d, "")
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, e = d.Token()
			return a, e
		}
		return nil, errors.New("unexpected delimiter")
	case json.Number:
		n, e := strconv.ParseFloat(string(x), 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n == 0 && strings.HasPrefix(string(x), "-") {
			return nil, errors.New("invalid protocol number")
		}
		if _, e = Canonical(n); e != nil {
			return nil, e
		}
		switch key {
		case "x", "y", "width", "height", "scrollX", "scrollY", "zoom", "scale", "deltaX", "deltaY":
			return n, nil
		}
		if math.Trunc(n) != n {
			return nil, errors.New("integer field required")
		}
		if key == "version" {
			return int(n), nil
		}
		return int64(n), nil
	case string:
		if _, e = Canonical(x); e != nil {
			return nil, e
		}
		return x, nil
	case bool, nil:
		return x, nil
	}
	return nil, errors.New("unsupported protocol value")
}
