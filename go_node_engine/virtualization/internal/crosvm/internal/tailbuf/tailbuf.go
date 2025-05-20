package tailbuf

import "io"

type TailBuffer struct {
	buf []byte // circular storage
	pos uint   // total bytes (virtually) written so far
}

func NewTailBuffer(capacity int) *TailBuffer {
	return &TailBuffer{buf: make([]byte, capacity)}
}

func (t *TailBuffer) Reset() {
	t.pos = 0
}

func (t *TailBuffer) Write(data []byte) (int, error) {
	dataLen := len(data)
	if dataLen == 0 {
		return 0, nil
	}
	uDataLen := uint(dataLen)

	capacity := uint(len(t.buf))
	newPos := t.pos + uDataLen

	var tailData []byte
	if uDataLen > capacity {
		tailData = data[uDataLen-capacity:]
	} else {
		tailData = data
	}

	start := (newPos - uint(len(tailData))) % capacity
	end := newPos % capacity

	if end > start {
		copy(t.buf[start:end], tailData)
	} else {
		split := capacity - start
		copy(t.buf[start:], tailData[:split])
		copy(t.buf[:end], tailData[split:])
	}

	t.pos = newPos

	return dataLen, nil
}

func (t *TailBuffer) WriteTo(w io.Writer) (int64, error) {
	return t.WriteToSkippingUntil(w, func(_ byte) bool { return true })
}

func (t *TailBuffer) WriteToSkippingUntil(w io.Writer, predicate func(b byte) bool) (int64, error) {
	capacity := uint(len(t.buf))

	if t.pos < capacity {
		return writeToDropUntil(t.buf[:t.pos], w, predicate)
	}

	end := t.pos % capacity
	if end == 0 {
		return writeToDropUntil(t.buf, w, predicate)
	}

	n1, err := writeToDropUntil(t.buf[end:capacity], w, predicate)
	if err != nil {
		return n1, err
	}

	// if the previous writeToDropUntil call wrote nothing,
	// we need to continue checking the predicate
	if n1 == 0 {
		return writeToDropUntil(t.buf[0:end], w, predicate)
	}

	n2, err := w.Write(t.buf[0:end])
	return n1 + int64(n2), err
}

func writeToDropUntil(data []byte, w io.Writer, predicate func(b byte) bool) (int64, error) {
	var startIdx = 0
	for _, b := range data {
		if predicate(b) {
			break
		}
		startIdx++
	}

	if startIdx >= len(data) {
		return 0, nil
	}

	n, err := w.Write(data[startIdx:])
	return int64(n), err
}
