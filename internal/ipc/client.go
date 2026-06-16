package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	ErrNotConnected = errors.New("not connected to intelligence daemon")
	ErrTimeout      = errors.New("request timeout")
)

type Client struct {
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	pending    map[int64]chan Message
	mu         sync.Mutex
	nextID     int64
	session    string
	notifyCh   chan Message
	closed     bool
	closeMu    sync.Mutex
}

func Dial(socketPath, session string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", socketPath, err)
	}

	c := &Client{
		conn:     conn,
		reader:   bufio.NewReader(conn),
		writer:   bufio.NewWriter(conn),
		pending:  make(map[int64]chan Message),
		session:  session,
		notifyCh: make(chan Message, 32),
	}

	go c.readLoop()
	return c, nil
}

func (c *Client) readLoop() {
	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			c.closeAll(err)
			return
		}

		msg, err := UnmarshalMessage(line)
		if err != nil {
			continue
		}

		base := msg.GetBase()
		if base.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[base.ID]
			if ok {
				delete(c.pending, base.ID)
			}
			c.mu.Unlock()
			if ok {
				select {
				case ch <- msg:
				default:
				}
			}
		} else {
			select {
			case c.notifyCh <- msg:
			default:
			}
		}
	}
}

func (c *Client) Request(ctx context.Context, req Message) (Message, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}
	c.nextID++
	id := c.nextID
	req.GetBase().ID = id
	req.GetBase().Session = c.session
	ch := make(chan Message, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	data = append(data, '\n')

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrNotConnected
	}
	if _, err := c.writer.Write(data); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	if err := c.writer.Flush(); err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ErrTimeout
	case <-time.After(5 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ErrTimeout
	}
}

func (c *Client) Notify() <-chan Message {
	return c.notifyCh
}

func (c *Client) SendAsync(req Message) error {
	req.GetBase().Session = c.session
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrNotConnected
	}
	_, err = c.writer.Write(data)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *Client) closeAll(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.pending {
		select {
		case ch <- &ErrorResponse{BaseMessage: BaseMessage{Type: MsgError}, Message: err.Error()}:
		default:
		}
	}
	c.pending = nil
}