package utils

import (
	"sync"
)

type genericSlice[T comparable] struct {
	data  []T
	mutex sync.RWMutex
}

type Slice[T comparable] interface {
	Add(elem T)
	RemoveElem(elem T)
	Remove(index int)
	Find(elem T) int
	Get() []T
	Exists(elem T) bool
}

func NewSlice[T comparable]() Slice[T] {
	return &genericSlice[T]{
		data: make([]T, 0),
	}
}

func (s *genericSlice[T]) Add(elem T) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.data = append(s.data, elem)
}

func (s *genericSlice[T]) Remove(index int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.removeLocked(index)
}

func (s *genericSlice[T]) RemoveElem(elem T) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.removeLocked(s.findLocked(elem))
}

func (s *genericSlice[T]) Find(elem T) int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.findLocked(elem)
}

func (s *genericSlice[T]) Exists(elem T) bool {
	return s.Find(elem) >= 0
}

func (s *genericSlice[T]) Get() []T {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	result := make([]T, len(s.data))
	copy(result, s.data)
	return result
}

// caller must hold at least a read lock
func (s *genericSlice[T]) findLocked(elem T) int {
	for index, currentElem := range s.data {
		if currentElem == elem {
			return index
		}
	}
	return -1
}

// caller must hold the write lock; swaps the target with the last element to avoid shifting
func (s *genericSlice[T]) removeLocked(index int) {
	if index >= 0 && index < len(s.data) {
		s.data[index] = s.data[len(s.data)-1]
		s.data = s.data[:len(s.data)-1]
	}
}
