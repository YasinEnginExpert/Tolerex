package storage

import (
	"os"
	"path/filepath"
	"strconv"
)

func WriteMessage(id int, text string) error {
	dir := "message"
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	filename := filepath.Join(dir, strconv.Itoa(id)+".msg")
	return os.WriteFile(filename, []byte(text), 0644)
}
