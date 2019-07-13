package util

import (
	"encoding/json"
	"io"
	"os"
)

func Save(obj interface{}, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(obj)
}

func SaveToFile(obj interface{}, filename string) error {

	fd, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer fd.Close()

	return Save(obj, fd)
}

func Load(obj interface{}, in io.Reader) error {
	dec := json.NewDecoder(in)
	return dec.Decode(obj)
}

func LoadFromFile(obj interface{}, filename string) error {
	fd, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fd.Close()
	return Load(obj, fd)
}

// func LoadFromFilesystem(obj interface{}, fs http.FileSystem, filename string) error {
// 	file, err := fs.Open(filename)
// 	if err != nil {
// 		return err
// 	}
// 	defer file.Close()
// 	return Load(obj, file)
// }
