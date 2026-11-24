package finlight

import (
	"bytes"
	"encoding/json"
)

// Field holds a key whose presence depends on the plan or on include. It has
// three states: absent (the key was not in the JSON), null (the key was there
// with null) and a value. An absent key is never turned into a zero value: an
// absent Companies field means "not delivered", while an empty list means
// "tagged, no company".
//
// The zero Field is absent. json.Marshal leaves absent fields out (the struct
// tags use omitzero) and writes null as null.
type Field[T any] struct {
	state fieldState
	value T
}

type fieldState uint8

const (
	fieldAbsent fieldState = iota
	fieldNull
	fieldValue
)

func fieldOf[T any](v T) Field[T] { return Field[T]{state: fieldValue, value: v} }

func fieldNullOf[T any]() Field[T] { return Field[T]{state: fieldNull} }

// Present reports whether the key was in the JSON (null counts as present).
func (f Field[T]) Present() bool { return f.state != fieldAbsent }

// IsNull reports whether the key was present with the value null.
func (f Field[T]) IsNull() bool { return f.state == fieldNull }

// Get returns the value and true when the key carried a value; otherwise the
// zero value and false (absent or null).
func (f Field[T]) Get() (T, bool) { return f.value, f.state == fieldValue }

// Value returns the value, or the zero value when absent or null.
func (f Field[T]) Value() T { return f.value }

// IsZero reports whether the field is absent. It lets json.Marshal drop absent
// fields through the omitzero tag option.
func (f Field[T]) IsZero() bool { return f.state == fieldAbsent }

// MarshalJSON writes the value, or null for a null field. Absent fields are
// skipped by their omitzero tag.
func (f Field[T]) MarshalJSON() ([]byte, error) {
	if f.state != fieldValue {
		return []byte("null"), nil
	}
	return json.Marshal(f.value)
}

// UnmarshalJSON reads null as a null field and anything else as a value.
func (f *Field[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		var zero T
		f.state, f.value = fieldNull, zero
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.state, f.value = fieldValue, v
	return nil
}
