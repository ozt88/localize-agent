package shared

import "strings"

type MultiFlag []string

func (m *MultiFlag) String() string { return strings.Join(*m, ",") }

func (m *MultiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
