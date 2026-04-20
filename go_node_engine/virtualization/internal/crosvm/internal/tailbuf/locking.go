package tailbuf

import (
	"io"
	"sync"
)

type LockingTailBuffer struct {
	TailBuffer
	mutex sync.RWMutex
}

func NewLockingTailBuffer(capacity int) *LockingTailBuffer {
	return &LockingTailBuffer{
		TailBuffer: TailBuffer{
			buf: make([]byte, capacity),
		},
		mutex: sync.RWMutex{},
	}
}

func (l *LockingTailBuffer) Reset() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.TailBuffer.Reset()
}

func (l *LockingTailBuffer) Write(data []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.TailBuffer.Write(data)
}

func (l *LockingTailBuffer) WriteTo(w io.Writer) (int64, error) {
	return l.WriteToSkippingUntil(w, func(_ byte) bool { return true })
}

func (l *LockingTailBuffer) WriteToSkippingUntil(w io.Writer, predicate func(b byte) bool) (int64, error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	return l.TailBuffer.WriteToSkippingUntil(w, predicate)
}

func (l *LockingTailBuffer) Capacity() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	return l.TailBuffer.Capacity()
}
