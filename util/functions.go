package util

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

// PathSplit is a path.Split alteranative that supports windows as well as linux
func PathSplit(pathStr string) (string, string) {
	i := strings.LastIndex(pathStr, string(os.PathSeparator))
	return pathStr[:i+1], pathStr[i+1:]
}

type AsyncCallResponse struct {
	Body  []byte
	Url   string
	Error error
}

// AsyncHttpGets gets several urls asynchronously
func AsyncHttpGets(urls []string) []AsyncCallResponse {
	ch := make(chan AsyncCallResponse, len(urls)) // buffered
	responses := []AsyncCallResponse{}

	for _, url := range urls {
		go func(url string) {
			timeout := time.Duration(2 * time.Second)
			client := http.Client{
				Timeout: timeout,
			}
			fmt.Printf("Fetching %s \n", url)
			resp, err := client.Get(url)
			if err != nil {
				ch <- AsyncCallResponse{Body: []byte{}, Url: url, Error: err}
				return
			}
			body, err := ioutil.ReadAll(resp.Body)
			ch <- AsyncCallResponse{Body: body, Url: url, Error: err}
		}(url)
	}

	for {
		select {
		case r := <-ch:
			fmt.Printf("%s was fetched\n", r.Url)
			responses = append(responses, r)
			if len(responses) == len(urls) {
				return responses
			}
			// case <-time.After(50 * time.Millisecond):
			// 	fmt.Printf(".")
		}
	}

	return responses

}
