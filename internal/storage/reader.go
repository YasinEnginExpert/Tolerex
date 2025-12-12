package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

func ReadMessage(baseDir string, id int) (string, error) {
	filename := filepath.Join(baseDir, "messages", strconv.Itoa(id)+".msg")
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("NOT_FOUND")
		}
		return "", err
	}
	return string(data), nil
}
