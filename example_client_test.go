package sse_test

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rndz/sse"
)

func Example() {
	s := &Server{}
	go runClient()
	log.Fatal("HTTP server error: ", http.ListenAndServe("localhost:3000", s))
}

type Server struct {
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	for {
		fmt.Fprintf(rw, "event: ts\ndata: %s\n\n", time.Now())
		fmt.Fprintf(rw, "event: new\ndata: %s\n\n", time.Now())
		flusher.Flush()
		time.Sleep(time.Second * 2)
		return
	}
}

func runClient() {
	time.Sleep(1 * time.Second)
	cfg := &sse.Config{
		URL: "http://localhost:3000",
	}
	client, err := sse.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	client.AddListener("ts", func(e sse.Event) {
		fmt.Printf("%+v\n", e)
	})
	client.AddListener("error", func(e sse.Event) {
		fmt.Println("error ", e)
	})
	client.AddListener("open", func(e sse.Event) {
		fmt.Println("open ", e)
	})
	client.Connect()
}
