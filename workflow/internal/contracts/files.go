package contracts

type FileStore interface {
	ReadJSON(path string, out any) error
	WriteJSON(path string, v any) error
	ReadLines(path string) ([]string, error)
	WriteLines(path string, lines []string) error
}
