package storage

import (
	"os"
	"path/filepath"
	"strconv"
)

func WriteMessage(baseDir string, id int, text string) error {
	dir := filepath.Join(baseDir, "messaage")
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	filename := filepath.Join(dir, strconv.Itoa(id)+".msg")
	return os.WriteFile(filename, []byte(text), 0644)
}
