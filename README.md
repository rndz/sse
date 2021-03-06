# Go library for SSE (Server-Sent Events) client

[![GoDoc](https://godoc.org/github.com/rndz/sse?status.svg)](http://godoc.org/github.com/rndz/sse)

Implementation of Server-Sent Events [EventSource][eventsource] interface.
Still work in progress - API might break!

Simple usage:
```go
package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/rndz/sse"
)

func main() {
	cfg := &sse.Config{
		URL: "http://example.com/chat",
	}
	client, err := sse.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	client.AddListener("join", func(e sse.Event) {
		fmt.Printf("%s has joined the conversation\n", e.Data)
	})
	client.AddListener("leave", func(e sse.Event) {
		fmt.Printf("%s has left the conversation\n", e.Data)
	})
	client.AddListener("error", func(e sse.Event) {
		log.Printf("error in sse stream: %s", e.Data)
	})
	client.Connect()
	runtime.Goexit()
}
```

[eventsource]: https://www.w3.org/TR/eventsource/
