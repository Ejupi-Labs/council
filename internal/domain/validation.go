package domain

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func ValidID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func ParseRevision(s string) (int64, *Error) {
	if len(s) == 0 || len(s) > 19 || s[0] == '0' {
		return 0, Invalid("revision decimal string")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, Invalid("revision decimal string")
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, Invalid("revision decimal string")
	}
	return n, nil
}
func StrictDecode(body []byte, target any) *Error {
	if len(body) > MaxBodyBytes {
		return Fail("input_too_large", "The request body exceeds 32 MiB. Reduce it and submit again.")
	}
	if !utf8.Valid(body) {
		return Fail("invalid_encoding", "Send valid UTF-8 JSON without malformed Unicode.")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Invalid("JSON object body")
	}
	// jsontext validates Unicode scalar values and duplicate object names at all depths.
	decoder := jsontext.NewDecoder(bytes.NewReader(body))
	if _, err := decoder.ReadValue(); err != nil {
		return Fail("invalid_encoding", "Send valid JSON with unique members and valid Unicode.")
	}
	if _, err := decoder.ReadValue(); err != io.EOF {
		return Invalid("single JSON object body")
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(body, &members); err != nil {
		return Invalid("JSON object body")
	}
	for _, value := range members {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Invalid("non-null JSON members")
		}
	}
	if err := json.Unmarshal(body, target, json.RejectUnknownMembers(true)); err != nil {
		return Invalid("documented JSON members and types")
	}
	return nil
}
func ValidateText(text string) *Error {
	if !utf8.ValidString(text) {
		return Fail("invalid_encoding", "Supply valid UTF-8 task text.")
	}
	if len(text) > MaxTextBytes {
		return Fail("input_too_large", "Task text exceeds 4 MiB. Reduce it explicitly and submit again.")
	}
	for _, r := range text {
		if !unicode.IsSpace(r) {
			return nil
		}
	}
	return Invalid("non-whitespace task text")
}
func ValidateQuery(query map[string]string, allowed ...string) *Error {
	for key := range query {
		found := false
		for _, a := range allowed {
			if key == a {
				found = true
				break
			}
		}
		if !found {
			return Invalid("documented query parameters")
		}
	}
	return nil
}
func ParseLimit(s string) (int, *Error) {
	if s == "" {
		return 25, nil
	}
	if len(s) > 3 || strings.Trim(s, "0123456789") != "" || s[0] == '0' {
		return 0, Invalid("limit from 1 through 100")
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || n > 100 {
		return 0, Invalid("limit from 1 through 100")
	}
	return n, nil
}

// MutationIdentity retains a valid caller identity even when another member is invalid.
func MutationIdentity(body []byte) string {
	if len(body) <= MaxBodyBytes && utf8.Valid(body) {
		var members map[string]jsontext.Value
		if json.Unmarshal(body, &members) == nil {
			var id string
			if json.Unmarshal(members["requestId"], &id) == nil && ValidID(id) {
				return id
			}
		}
	}
	return NewID()
}
