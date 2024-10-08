package util

type empty struct{}
type Set[T comparable] map[T]empty

var member empty

func (s Set[T]) Add(elem T) bool {
	if _, exists := s[elem]; exists {
		return false
	}

	s[elem] = member
	return true
}

func (s Set[T]) Delete(elem T) bool {
	if _, exists := s[elem]; exists {
		delete(s, elem)
		return true
	}
	return false
}

func (s Set[T]) IsMember(elem T) bool {
	_, exists := s[elem]
	return exists
}

func (s Set[T]) Elems() []T {
	elems := make([]T, len(s))
	idx := 0
	for e := range s {
		elems[idx] = e
		idx++
	}
	return elems
}
