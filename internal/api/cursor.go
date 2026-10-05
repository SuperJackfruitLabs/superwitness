package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// A cursor is an opaque, URL-safe offset. A later run index can change what is inside.
type cursor struct {
	Offset int `json:"o"`
}

func encodeCursor(off int) string {
	b, _ := json.Marshal(cursor{Offset: off})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, errors.New("cursor is not one this API issued")
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.Offset < 0 {
		return 0, errors.New("cursor is not one this API issued")
	}
	return c.Offset, nil
}
