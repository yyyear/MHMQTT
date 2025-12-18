package tools

import (
	"strings"
	"sync"
)

var builder sync.Pool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// GetStringBuilder 获取字符串构建器
func GetStringBuilder() *strings.Builder {
	b := builder.Get().(*strings.Builder)
	return b
}

// PutStringBuilder 归还字符串构建器
func PutStringBuilder(b *strings.Builder) {
	b.Reset()
	builder.Put(b)
}
