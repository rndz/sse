package sse

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type state uint8

const (
	stateConnecting state = 0
	stateOpen       state = 1
	stateClosed     state = 2
)

// Client is main struct that handles connecting/streaming/etc...
type Client struct {
	cl        *http.Client
	req       *http.Request
	r         io.ReadCloser
	last      string
	retry     time.Duration
	eventChan chan Event
	retryChan chan time.Duration
	stopChan  chan struct{}

	add       chan listener
	listeners map[string]func(Event)

	sync.RWMutex
	readyState state
}

type listener struct {
	name string
	f    func(Event)
}

// Event is SSE event data represenation
type Event struct {
	ID    string
	Event string
	Data  string
}

// Config is a struct used to define and override default parameters:
//   Client - *http.Client, if non provided, http.DefaultClient will be used.
//   URL    - URL of SSE stream to connect to. Must be provided. No default
//            value.
//   Retry  - time.Duration of how long should SSE client wait before trying
//            to reconnect after disconnection. Default is 2 seconds.
type Config struct {
	Client *http.Client
	URL    string
	Retry  time.Duration
}

// New creates a client based on a passed Config.
func New(cfg *Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("sse: URL config option MUST be provided")
	}
	req, err := http.NewRequest("GET", cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("sse: could not make request to %s: %v", cfg.URL, err)
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept", "text/event-stream")
	retry := 2 * time.Second
	if cfg.Retry != 0 {
		retry = cfg.Retry
	}
	client := &Client{
		readyState: stateConnecting,
		req:        req,
		retry:      retry,
		eventChan:  make(chan Event, 1),
		retryChan:  make(chan time.Duration, 1),
		stopChan:   make(chan struct{}),
		add:        make(chan listener),
		listeners:  make(map[string]func(Event)),
	}
	client.cl = http.DefaultClient
	if cfg.Client != nil {
		client.cl = cfg.Client
	}
	go client.run()
	return client, nil
}

// Connect connects to given SSE endpoint and starts reading the stream and
// transmitting events.
func (c *Client) Connect() {
	go c.connect()
}

// AddListener adds a listener for a given event type. A listener is simple
// callback function that passes Event struct.
func (c *Client) AddListener(event string, f func(Event)) {
	c.add <- listener{
		name: event,
		f:    f,
	}
}

// Stop stops the client of accepting any more stream requests.
func (c *Client) Stop() {
	c.stop()
}

func (c *Client) stop() {
	close(c.stopChan)
	c.Lock()
	c.readyState = stateClosed
	c.Unlock()
}

func (c *Client) run() {
	for {
		select {
		case l := <-c.add:
			c.listeners[l.name] = l.f
		case event := <-c.eventChan:
			f, ok := c.listeners[event.Event]
			if !ok {
				continue
			}
			if event.ID != "" {
				c.last = event.ID
			}
			f(event)
		case val := <-c.retryChan:
			c.retry = val
		case <-c.stopChan:
			return
		}
	}
}

func (c *Client) read() {
	c.RLock()
	state := c.readyState
	c.RUnlock()
	switch state {
	case stateOpen:
	case stateConnecting:
		return
	case stateClosed:
		return
	default:
		return
	}
	c.decode()
}

func (c *Client) decode() {
	defer c.r.Close()
	dec := bufio.NewReader(c.r)
	_, err := dec.Peek(1)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	if err != nil {
		c.fireErrorAndRecover(err)
		return
	}
	for {
		event := new(Event)
		event.Event = "message"
		for {
			line, err := dec.ReadString('\n')
			if err != nil {
				c.fireErrorAndRecover(err)
				return
			}
			if line == "\n" {
				break
			}
			line = strings.TrimSuffix(line, "\n")
			if strings.HasPrefix(line, ":") {
				continue
			}
			sections := strings.SplitN(line, ":", 2)
			field, value := sections[0], ""
			if len(sections) == 2 {
				value = strings.TrimPrefix(sections[1], " ")
			}
			switch field {
			case "event":
				event.Event = value
			case "data":
				event.Data += value + "\n"
			case "id":
				event.ID = value
			case "retry":
				retry, err := strconv.Atoi(value)
				if err != nil {
					break
				}
				c.retryChan <- time.Duration(retry) * time.Millisecond
			}
		}
		event.Data = strings.TrimSuffix(event.Data, "\n")
		c.eventChan <- *event
	}
}

func (c *Client) connect() {
	c.req.Header.Set("Last-Event-ID", c.last)
	resp, err := c.cl.Do(c.req)
	if err != nil {
		go c.connect()
		return
	}
	// TODO: check other status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// TODO: check content-type
	case http.StatusNoContent:
		c.stop()
		return
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		c.reconnect()
		return
	default:
		c.stop()
		return
	}
	c.r = resp.Body
	c.fireOpen()
}

func (c *Client) reconnect() {
	c.Lock()
	c.readyState = stateConnecting
	c.Unlock()
	time.Sleep(c.retry)
	// TODO: also implement exponential backoff delay
	go c.connect()
}

func (c *Client) fireOpen() {
	c.Lock()
	c.readyState = stateOpen
	c.Unlock()
	go c.read()
	event := new(Event)
	event.Event = "open"
	c.eventChan <- *event
}

func (c *Client) fireErrorAndRecover(err error) {
	if err != io.EOF {
		event := new(Event)
		event.Event = "error"
		event.Data = err.Error()
		c.eventChan <- *event
	}
	c.reconnect()
}
