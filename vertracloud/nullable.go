package vertracloud

import "encoding/json"

// Nullable distinguishes an omitted request field from an explicit JSON null.
type Nullable[T any] struct {
	Value *T
}

func NullableValue[T any](value T) *Nullable[T] { return &Nullable[T]{Value: &value} }

func NullableNull[T any]() *Nullable[T] { return &Nullable[T]{} }

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*n.Value)
}
