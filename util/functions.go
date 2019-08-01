package util

import (
	"os"
	"strings"
)

func PathSplit(pathStr string) (string, string) {
	i := strings.LastIndex(pathStr, string(os.PathSeparator))
	return pathStr[:i+1], pathStr[i+1:]
}
