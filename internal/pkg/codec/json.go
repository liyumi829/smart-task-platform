// internal/pkg/codec/json.go
// Package codec
// 实现 Json 的通用序列化和反序列化
package codec

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

type JSONCodec struct {
	// UseNumber 防止大整数反序列化到 interface{} 时精度丢失。
	UseNumber bool

	// DisableHTMLEscape 禁用 HTML 字符转义。
	//
	// encoding/json 默认会把 <, >, & 转义成 \u003c, \u003e, \u0026。
	DisableHTMLEscape bool

	// DisallowUnknownFields 反序列化时禁止未知字段。
	//
	// 通常 API 请求解析可以开启，缓存反序列化不建议开启。
	DisallowUnknownFields bool

	// DisallowTrailingData 禁止 JSON 后面带有额外数据。
	//
	// 例如 {"id":1} garbage 会返回错误。
	DisallowTrailingData bool
}

const maxPooledBufferSize = 256 << 10 // 256KB

var bufferPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

func (c JSONCodec) Marshal(v any) ([]byte, error) {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	defer func() {
		if buf.Cap() <= maxPooledBufferSize {
			bufferPool.Put(buf)
		}
	}()

	encoder := json.NewEncoder(buf)
	if c.DisableHTMLEscape {
		encoder.SetEscapeHTML(false)
	}

	if err := encoder.Encode(v); err != nil {
		return nil, err
	}

	b := buf.Bytes()

	// json.Encoder.Encode 会在结尾追加 '\n'。
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}

	out := make([]byte, len(b)) // 分配好空间
	copy(out, b)                // 拷贝，防止指向同一个buffer byte切片

	return out, nil
}

func (c JSONCodec) Unmarshal(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))

	if c.UseNumber {
		decoder.UseNumber()
	}

	if c.DisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}

	if err := decoder.Decode(v); err != nil {
		return err
	}

	if c.DisallowTrailingData { // 如果开启了追加数据解析
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return ErrJsonUnpxpectedTralingData
			}
			return err
		}
	}

	return nil
}
