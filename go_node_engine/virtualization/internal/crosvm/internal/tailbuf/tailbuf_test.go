package tailbuf_test

import (
	"bytes"
	"go_node_engine/virtualization/internal/crosvm/internal/tailbuf"
	"gotest.tools/v3/assert"
	"testing"
)

func TestEmpty(t *testing.T) {
	tb := tailbuf.NewTailBuffer(8)

	assertContentsEqual(t, tb, []byte{})
}

func TestNoWrapHalf(t *testing.T) {
	tb := tailbuf.NewTailBuffer(10)

	assertWrite(t, tb, []byte("hello"))

	assertContentsEqual(t, tb, []byte("hello"))
}

func TestNoWrapFull(t *testing.T) {
	tb := tailbuf.NewTailBuffer(5)

	assertWrite(t, tb, []byte("hello"))

	assertContentsEqual(t, tb, []byte("hello"))
}

func TestMultiWriteSingleWrap(t *testing.T) {
	tb := tailbuf.NewTailBuffer(4)

	assertWrite(t, tb, []byte("abcd")) // buffer = abcd
	assertWrite(t, tb, []byte("ef"))   // buffer = cdef

	assertContentsEqual(t, tb, []byte("cdef"))
}

func TestSingleWriteMultiWrap(t *testing.T) {
	tb := tailbuf.NewTailBuffer(4)

	assertWrite(t, tb, []byte("abcdefghijklmnopqrstuvwxyz"))

	assertContentsEqual(t, tb, []byte("wxyz"))
}

func TestMultiWriteMultiWrap(t *testing.T) {
	tb := tailbuf.NewTailBuffer(5)

	assertWrite(t, tb, []byte("12"))   // 12
	assertWrite(t, tb, []byte("345"))  // 12345
	assertWrite(t, tb, []byte("6"))    // 23456
	assertWrite(t, tb, []byte("7890")) // 67890
	assertWrite(t, tb, []byte("34"))   // 89034

	assertContentsEqual(t, tb, []byte("89034"))
}

func TestSmallThenLargeWriteWrap(t *testing.T) {
	tb := tailbuf.NewTailBuffer(5)

	assertWrite(t, tb, []byte("12"))                         // 12
	assertWrite(t, tb, []byte("345"))                        // 12345
	assertWrite(t, tb, []byte("6"))                          // 23456
	assertWrite(t, tb, []byte("7890"))                       // 67890
	assertWrite(t, tb, []byte("abcdefghijklmnopqrstuvwxyz")) // vwxyz

	assertContentsEqual(t, tb, []byte("vwxyz"))
}

func TestMultiWrapDivisible(t *testing.T) {
	tb := tailbuf.NewTailBuffer(4)

	assertWrite(t, tb, []byte("12341234123412341234")) // 1234

	assertContentsEqual(t, tb, []byte("1234"))
}

func assertWrite(t *testing.T, tb *tailbuf.TailBuffer, data []byte) {
	size, err := tb.Write(data)
	assert.NilError(t, err)
	assert.Equal(t, len(data), size)
}

func assertContentsEqual(t *testing.T, tb *tailbuf.TailBuffer, data []byte) {
	buf := &bytes.Buffer{}
	size, err := tb.WriteTo(buf)
	assert.NilError(t, err)
	if len(data) > 0 {
		assert.DeepEqual(t, buf.Bytes(), data)
	}
	assert.Equal(t, len(data), int(size))
}
