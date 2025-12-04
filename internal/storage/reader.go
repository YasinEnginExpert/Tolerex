package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

func ReadMessage(id int) (string, error) {
	filename := filepath.Join("messages", strconv.Itoa(id)+".mgs")
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("NOT_FOUND")
		}
		return "", err
	}
	return string(data), nil

}
