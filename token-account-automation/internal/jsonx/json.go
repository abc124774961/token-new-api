package jsonx

import (
	"encoding/json"
	"io"
)

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func Decode(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}
