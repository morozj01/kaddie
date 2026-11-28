package jsondoc_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/morozj01/kaddie/pkg/proposal/internal/jsondoc"
)

func TestDecodeRejectsARepeatedKeyAtAnyDepth(t *testing.T) {
	for name, body := range map[string]string{
		"top level": `{"a":1,"a":2}`,
		"nested":    `{"x":{"y":[{"a":1,"a":null}]}}`,
		"escaped":   `{"spec":{},"spec":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			var dup *jsondoc.DuplicateKeyError
			if _, err := jsondoc.Decode([]byte(body)); !errors.As(err, &dup) {
				t.Errorf("Decode(%s) error = %v, want a DuplicateKeyError", body, err)
			}
		})
	}
}

func TestDecodeReadsWhatEncodingJSONReads(t *testing.T) {
	got, err := jsondoc.Decode([]byte(` {"a":[1,2.50,"s",true,null,{}],"b":{"A":"x"},"c":12345678901234567890} `))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"a": []any{json.Number("1"), json.Number("2.50"), "s", true, nil, map[string]any{}},
		"b": map[string]any{"A": "x"},
		"c": json.Number("12345678901234567890"),
	}
	if g, w := marshal(t, got), marshal(t, want); g != w {
		t.Errorf("Decode = %s, want %s", g, w)
	}
}

func TestDecodeRejectsMalformedDocuments(t *testing.T) {
	deep := strings.Repeat("[", 20000) + strings.Repeat("]", 20000)
	for name, body := range map[string]string{
		"empty":     ``,
		"truncated": `{"a":`,
		"trailing":  `{"a":1} {"b":2}`,
		"comma":     `{"a":1,}`,
		"too deep":  deep,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := jsondoc.Decode([]byte(body)); err == nil {
				t.Errorf("Decode accepted %q", body[:min(len(body), 20)])
			}
		})
	}
}

func TestStringsIncludesTheKeysAndValuesOfRepeatedKeys(t *testing.T) {
	got, err := jsondoc.Strings([]byte(`{"spec":{"image":"<redacted:x>"},"spec":null,"key":["v"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"spec", "image", "<redacted:x>", "spec", "key", "v"}; !slices.Equal(got, want) {
		t.Errorf("Strings = %q, want %q", got, want)
	}
	if _, err := jsondoc.Strings([]byte(`{"a":"b"`)); err == nil {
		t.Error("Strings accepted a truncated document")
	}
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
