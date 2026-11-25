package finlight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The decoders in this file read wire JSON with exact, case-sensitive key
// matching (encoding/json alone would match keys case-insensitively), report
// whether optional keys were present, and never coerce types.

type jsonObject map[string]json.RawMessage

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func decodeErr(path string, raw json.RawMessage, format string, args ...any) *DecodeError {
	r := string(raw)
	if len(r) > 200 {
		r = r[:200] + "..."
	}
	return &DecodeError{Path: path, Raw: r, Message: fmt.Sprintf(format, args...)}
}

func jsonKind(raw json.RawMessage) string {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return "nothing"
	}
	switch t[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	}
	return "number"
}

// parseObject reads a JSON object. A missing or null object is an error.
func parseObject(raw json.RawMessage, path string) (jsonObject, error) {
	if jsonKind(raw) != "object" {
		return nil, decodeErr(path, raw, "expected an object, got %s", jsonKind(raw))
	}
	var o jsonObject
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, &DecodeError{Path: path, Message: "invalid JSON object", Err: err}
	}
	return o, nil
}

func sub(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func (o jsonObject) reqString(path, key string) (string, error) {
	raw, ok := o[key]
	if !ok {
		return "", decodeErr(sub(path, key), nil, "required key is missing")
	}
	if jsonKind(raw) != "string" {
		return "", decodeErr(sub(path, key), raw, "expected a string, got %s", jsonKind(raw))
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", &DecodeError{Path: sub(path, key), Message: "invalid string", Err: err}
	}
	return s, nil
}

// optString reads a string|null key. Absent and null both read as nil.
func (o jsonObject) optString(path, key string) (*string, error) {
	raw, ok := o[key]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	s, err := o.reqString(path, key)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// plainString reads a string key; absent or null read as "".
func (o jsonObject) plainString(path, key string) (string, error) {
	s, err := o.optString(path, key)
	if err != nil || s == nil {
		return "", err
	}
	return *s, nil
}

func (o jsonObject) optFloat(path, key string) (*float64, error) {
	raw, ok := o[key]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	if jsonKind(raw) != "number" {
		return nil, decodeErr(sub(path, key), raw, "expected a number, got %s", jsonKind(raw))
	}
	f, err := strconv.ParseFloat(string(bytes.TrimSpace(raw)), 64)
	if err != nil {
		return nil, &DecodeError{Path: sub(path, key), Raw: string(raw), Message: "invalid number", Err: err}
	}
	return &f, nil
}

func parseInt64(raw json.RawMessage, path string) (int64, error) {
	if jsonKind(raw) != "number" {
		return 0, decodeErr(path, raw, "expected an integer, got %s", jsonKind(raw))
	}
	n, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	if err != nil {
		return 0, decodeErr(path, raw, "expected an integer")
	}
	return n, nil
}

func (o jsonObject) reqInt64(path, key string) (int64, error) {
	raw, ok := o[key]
	if !ok {
		return 0, decodeErr(sub(path, key), nil, "required key is missing")
	}
	return parseInt64(raw, sub(path, key))
}

func (o jsonObject) optInt64(path, key string) (*int64, error) {
	raw, ok := o[key]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	n, err := parseInt64(raw, sub(path, key))
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (o jsonObject) optBool(path, key string) (*bool, error) {
	raw, ok := o[key]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	if jsonKind(raw) != "boolean" {
		return nil, decodeErr(sub(path, key), raw, "expected a boolean, got %s", jsonKind(raw))
	}
	b := bytes.Equal(bytes.TrimSpace(raw), []byte("true"))
	return &b, nil
}

// array reads a JSON array as its raw elements. Absent and null read as nil
// with present=false.
func (o jsonObject) array(path, key string) (elems []json.RawMessage, present bool, err error) {
	raw, ok := o[key]
	if !ok || isJSONNull(raw) {
		return nil, ok, nil
	}
	if jsonKind(raw) != "array" {
		return nil, true, decodeErr(sub(path, key), raw, "expected an array, got %s", jsonKind(raw))
	}
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, true, &DecodeError{Path: sub(path, key), Message: "invalid array", Err: err}
	}
	if elems == nil {
		elems = []json.RawMessage{}
	}
	return elems, true, nil
}

// envelope reads the named list of a list response. The key is always there
// (contract section 6): a missing or null one is a *DecodeError, never an
// empty result.
func (o jsonObject) envelope(key string) ([]json.RawMessage, error) {
	raw, ok := o[key]
	if !ok {
		return nil, decodeErr(key, nil, "required key is missing")
	}
	elems, _, err := o.array("", key)
	if err != nil {
		return nil, err
	}
	if elems == nil {
		return nil, decodeErr(key, raw, "expected an array, got null")
	}
	return elems, nil
}

// stringList reads a string[] key; absent and null read as an empty list.
func (o jsonObject) stringList(path, key string) ([]string, error) {
	elems, _, err := o.array(path, key)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(elems))
	for i, e := range elems {
		if jsonKind(e) != "string" {
			return nil, decodeErr(fmt.Sprintf("%s[%d]", sub(path, key), i), e, "expected a string, got %s", jsonKind(e))
		}
		var s string
		if err := json.Unmarshal(e, &s); err != nil {
			return nil, &DecodeError{Path: fmt.Sprintf("%s[%d]", sub(path, key), i), Message: "invalid string", Err: err}
		}
		out = append(out, s)
	}
	return out, nil
}

// parseTime parses a wire timestamp strictly as RFC 3339 with Z or an offset.
func parseTime(s *string) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	return parseTimeString(*s)
}

func parseTimeString(s string) (time.Time, bool) {
	// time.RFC3339Nano needs the "T", seconds and a zone (Z or ±hh:mm) and
	// rejects lower case; a comma as decimal sign is refused here as well.
	if strings.ContainsRune(s, ',') {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
