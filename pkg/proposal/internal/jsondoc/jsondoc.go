// Package jsondoc reads the JSON bodies of proposals without losing what a
// map-based decode drops. The API server decodes a built-in kind into
// structs, which merge an object that repeats a key into its first
// occurrence, while a map keeps only the last; a body that repeats a key can
// mean two things, so it is an error here and every reader of a body shares
// this decoder.
package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// maxDepth matches encoding/json's nesting limit.
const maxDepth = 10000

// DuplicateKeyError is the error for an object that repeats a key.
type DuplicateKeyError struct{ Key string }

func (e *DuplicateKeyError) Error() string {
	return fmt.Sprintf("the object repeats the key %q", e.Key)
}

// Decode reads one JSON document as map[string]any, []any, string,
// json.Number, bool or nil. Numbers are kept as written. It fails on
// trailing data and on an object that repeats a key at any depth, comparing
// keys as decoded, so an escaped spelling of a key repeats it.
func Decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := value(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data after the JSON document")
	}
	return v, nil
}

// Strings returns every object key and string value in b, in order,
// including those of repeated keys that Decode refuses. It fails when b is not
// syntactically valid JSON.
func Strings(b []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	var out []string
	open := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if open > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch tok := tok.(type) {
		case string:
			out = append(out, tok)
		case json.Delim:
			if tok == '{' || tok == '[' {
				open++
			} else {
				open--
			}
		}
	}
}

func value(dec *json.Decoder, depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("the JSON document is nested too deeply")
	}
	tok, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if delim == '[' {
		list := []any{}
		for dec.More() {
			e, err := value(dec, depth+1)
			if err != nil {
				return nil, err
			}
			list = append(list, e)
		}
		_, err := dec.Token()
		return list, err
	}
	obj := map[string]any{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key := keyTok.(string)
		if _, repeated := obj[key]; repeated {
			return nil, &DuplicateKeyError{Key: key}
		}
		if obj[key], err = value(dec, depth+1); err != nil {
			return nil, err
		}
	}
	_, err = dec.Token()
	return obj, err
}
