package utils

import (
	"sync"
)

type stringSlice struct {
	slice []string
	mutex sync.Mutex
}

type StringSlice interface {
	Add(elem string)
	RemoveElem(elem string)
	Remove(index int)
	Find(elem string) int
	Get() []string
	Exists(elem string) bool
}

func NewStringSlice() StringSlice {
	return &stringSlice{
		slice: make([]string, 0),
		mutex: sync.Mutex{},
	}
}

func (s *stringSlice) Add(elem string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.slice = append(s.slice, elem)
}

func (s *stringSlice) Remove(index int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index >= 0 && index < len(s.slice) {
		s.slice[index] = s.slice[len(s.slice)-1]
		s.slice = s.slice[:len(s.slice)-1]
	}
}

func (s *stringSlice) RemoveElem(elem string) {
	position := s.Find(elem)
	s.Remove(position)
}

func (s *stringSlice) Find(elem string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for index, currentElem := range s.slice {
		if currentElem == elem {
			return index
		}
	}
	return -1
}

func (s *stringSlice) Exists(elem string) bool {
	return s.Find(elem) >= 0
}

func (s *stringSlice) Get() []string {
	return s.slice
}
