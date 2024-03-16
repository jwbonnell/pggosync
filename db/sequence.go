package db

import "fmt"

type Sequence struct {
	Schema string
	Name   string
	Column string
}

func (s *Sequence) FullName() string {
	return fmt.Sprintf("%s.%s", s.Schema, s.Name)
}

func (s *Sequence) Equal(other Sequence) bool {
	return s.Schema == other.Schema && s.Name == other.Name
}
