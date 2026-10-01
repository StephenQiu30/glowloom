package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
)

const maxFrameBytes = 48 * 1024 * 1024

type rpcFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type receivedFrame struct {
	frame rpcFrame
	err   error
}
type rpcClient struct {
	conn     io.ReadWriteCloser
	received chan receivedFrame
	stopped  chan struct{}
	readDone chan struct{}
	once     sync.Once
	nextID   int
	pending  []rpcFrame
}

func newRPC(conn io.ReadWriteCloser) *rpcClient {
	c := &rpcClient{conn: conn, received: make(chan receivedFrame, 16), stopped: make(chan struct{}), readDone: make(chan struct{})}
	go c.read()
	return c
}
func (c *rpcClient) read() {
	defer close(c.readDone)
	defer close(c.received)
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 64*1024), maxFrameBytes)
	for scanner.Scan() {
		var frame rpcFrame
		err := decodeJSON(scanner.Bytes(), &frame)
		if err != nil {
			err = errProtocol
		}
		select {
		case c.received <- receivedFrame{frame: frame, err: err}:
		case <-c.stopped:
			return
		}
		if err != nil {
			return
		}
	}
	if scanner.Err() != nil {
		select {
		case c.received <- receivedFrame{err: errProtocol}:
		case <-c.stopped:
		}
	}
}
func (c *rpcClient) close() { c.once.Do(func() { close(c.stopped); _ = c.conn.Close(); <-c.readDone }) }
func (c *rpcClient) send(ctx context.Context, value any) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > maxFrameBytes {
		return errProtocol
	}
	raw = append(raw, '\n')
	done := make(chan error, 1)
	go func() {
		n, err := c.conn.Write(raw)
		if err == nil && n != len(raw) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		c.close()
		<-done
		return ctx.Err()
	}
}
func (c *rpcClient) receive(ctx context.Context) (rpcFrame, error) {
	select {
	case <-ctx.Done():
		return rpcFrame{}, ctx.Err()
	case result, ok := <-c.received:
		if !ok {
			return rpcFrame{}, io.EOF
		}
		return result.frame, result.err
	}
}
func (c *rpcClient) call(ctx context.Context, method string, params, target any) error {
	c.nextID++
	id := c.nextID
	if err := c.send(ctx, map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		frame, err := c.receive(ctx)
		if err != nil {
			return err
		}
		if frame.Method != "" {
			// Never approve arbitrary tools or user-input requests from the server.
			if len(frame.ID) != 0 {
				return errProtocol
			}
			if frame.Method == "item/completed" || frame.Method == "turn/completed" {
				if len(c.pending) >= 64 {
					return errProtocol
				}
				c.pending = append(c.pending, frame)
			}
			continue
		}
		var responseID int
		if decodeJSON(frame.ID, &responseID) != nil || responseID != id {
			continue
		}
		if len(frame.Error) != 0 && !bytes.Equal(frame.Error, []byte("null")) {
			return errProtocol
		}
		return decodeJSON(frame.Result, target)
	}
}
func (c *rpcClient) event(ctx context.Context) (rpcFrame, error) {
	if len(c.pending) > 0 {
		event := c.pending[0]
		c.pending = c.pending[1:]
		return event, nil
	}
	for {
		event, err := c.receive(ctx)
		if err != nil {
			return rpcFrame{}, err
		}
		if event.Method != "" {
			if len(event.ID) != 0 {
				return rpcFrame{}, errProtocol
			}
			return event, nil
		}
	}
}
func decodeJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return errProtocol
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errProtocol
	}
	return nil
}
