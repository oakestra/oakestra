package utils

import "testing"

func TestStringSlice_Add(t *testing.T) {
	s := NewStringSlice()
	s.Add("test")
	slice := s.Get()
	for _, s := range slice {
		if s == "test" {
			return
		}
	}
	t.Error("Elem not added correctly")
}

func TestStringSlice_Exists(t *testing.T) {
	s := NewStringSlice()
	s.Add("test")
	if s.Exists("mario") {
		t.Error("mario should not be in the slice")
	}
	if !s.Exists("test") {
		t.Error("test should be in the slice")
	}
}

func TestStringSlice_Find(t *testing.T) {
	s := NewStringSlice()
	s.Add("0")
	s.Add("1")
	index := s.Find("0")
	if index != 0 {
		t.Error("Wrong indexing")
	}
	index = s.Find("3")
	if index != -1 {
		t.Error("3 is not part of the slice, therefore index should be -1")
	}
}

func TestStringSlice_Remove(t *testing.T) {
	s := NewStringSlice()
	s.Add("m")
	s.Remove(0)
	if len(s.Get()) > 0 {
		t.Error("Slice size must be 0 in this case")
	}
}

func TestStringSlice_Remove2(t *testing.T) {
	s := NewStringSlice()
	s.Add("a")
	s.Add("m")
	s.Add("n")
	s.Remove(1)
	if !s.Exists("n") {
		t.Error("wrong element removed")
	}
	if !s.Exists("a") {
		t.Error("wrong element removed")
	}
}

func TestStringSlice_RemoveElem(t *testing.T) {
	s := NewStringSlice()
	s.Add("a")
	s.Add("m")
	s.Add("n")
	s.RemoveElem("m")
	if !s.Exists("n") {
		t.Error("wrong element removed")
	}
	if !s.Exists("a") {
		t.Error("wrong element removed")
	}
}
