package util

import (
	"encoding/json"
	"io"
	"os"
)

// Save serializes an object to json
func Save(obj interface{}, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(obj)
}

// SaveToFile serializes an object to json and saves it to file
func SaveToFile(obj interface{}, filename string) error {

	fd, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer fd.Close()

	return Save(obj, fd)
}

// Load deserializes a JSON object
func Load(obj interface{}, in io.Reader) error {
	dec := json.NewDecoder(in)
	return dec.Decode(obj)
}

// LoadFromFile reads a json file into an object
func LoadFromFile(obj interface{}, filename string) error {
	fd, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fd.Close()
	return Load(obj, fd)
}
