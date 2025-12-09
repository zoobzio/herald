package herald

import "encoding/json"

// Codec defines the serialization contract for message payloads.
// Implement this interface to use alternative formats like Protobuf, MessagePack, or Avro.
type Codec interface {
	// Marshal serializes a value to bytes.
	Marshal(v any) ([]byte, error)

	// Unmarshal deserializes bytes into a value.
	Unmarshal(data []byte, v any) error

	// ContentType returns the MIME type for metadata propagation.
	ContentType() string
}

// JSONCodec implements Codec using encoding/json.
type JSONCodec struct{}

// Marshal serializes v to JSON bytes.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into v.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ContentType returns the JSON MIME type.
func (JSONCodec) ContentType() string {
	return "application/json"
}

// Ensure JSONCodec implements Codec.
var _ Codec = JSONCodec{}
