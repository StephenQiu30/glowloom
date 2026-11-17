package domain

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"strconv"
	"strings"
)

// CanonicalJSON preserves integer precision and identity across JSONB key/number normalization.
func CanonicalJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if dec.Decode(&tree) != nil || dec.Decode(new(any)) != io.EOF {
		return nil, ErrInvalid
	}
	var normalize func(any) (any, error)
	normalize = func(v any) (any, error) {
		switch x := v.(type) {
		case map[string]any:
			for k, value := range x {
				n, err := normalize(value)
				if err != nil {
					return nil, err
				}
				x[k] = n
			}
			return x, nil
		case []any:
			for i, value := range x {
				n, err := normalize(value)
				if err != nil {
					return nil, err
				}
				x[i] = n
			}
			return x, nil
		case json.Number:
			s := string(x)
			if len(s) > 4096 {
				return nil, ErrInvalid
			}
			if i := strings.IndexAny(s, "eE"); i >= 0 {
				ex, err := strconv.Atoi(s[i+1:])
				if err != nil || ex > 4096 || ex < -4096 {
					return nil, ErrInvalid
				}
			}
			r, ok := new(big.Rat).SetString(s)
			if !ok {
				return nil, ErrInvalid
			}
			den := new(big.Int).Set(r.Denom())
			two, five := 0, 0
			for den.Bit(0) == 0 {
				den.Rsh(den, 1)
				two++
			}
			q, rem := new(big.Int), new(big.Int)
			for {
				q.QuoRem(den, big.NewInt(5), rem)
				if rem.Sign() != 0 {
					break
				}
				den.Set(q)
				five++
			}
			if den.Cmp(big.NewInt(1)) != 0 {
				return nil, ErrInvalid
			}
			scale := max(two, five)
			if scale > 4096 {
				return nil, ErrInvalid
			}
			value := r.FloatString(scale)
			if strings.Contains(value, ".") {
				value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
			}
			if value == "-0" {
				value = "0"
			}
			return json.Number(value), nil
		default:
			return v, nil
		}
	}
	tree, err := normalize(tree)
	if err != nil {
		return nil, err
	}
	return json.Marshal(tree)
}
