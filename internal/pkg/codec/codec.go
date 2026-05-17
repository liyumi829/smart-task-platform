// internal/pkg/codec/codec.go
// Package codec
// 接口定义和提供的默认解析方法

package codec

type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

var defaultJSONCodec = JSONCodec{
	UseNumber:             false,
	DisableHTMLEscape:     true,
	DisallowUnknownFields: false,
	DisallowTrailingData:  true,
}

func Marshal(v any) ([]byte, error) {
	return defaultJSONCodec.Marshal(v)
}

func Unmarshal(data []byte, v any) error {
	return defaultJSONCodec.Unmarshal(data, v)
}

func MarshalString(v any) (string, error) {
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func UnmarshalString(data string, v any) error {
	return Unmarshal([]byte(data), v)
}
