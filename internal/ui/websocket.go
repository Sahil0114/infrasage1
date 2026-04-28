package ui

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Event represents a UI event sent over the websocket.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type TerminalPayload struct {
	Line  string `json:"line"`
	Level string `json:"level,omitempty"`
}

type PipelinePayload struct {
	Step string `json:"step"`
}

type ModePayload struct {
	Current      string `json:"current"`
	AWSAvailable bool   `json:"awsAvailable"`
}

type FindingPayload struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Message  string `json:"message,omitempty"`
	Resource string `json:"resource,omitempty"`
}

type ScanPayload struct {
	Tool     string           `json:"tool"`
	Passed   int              `json:"passed"`
	Failed   int              `json:"failed"`
	Status   string           `json:"status"`
	Duration string           `json:"duration,omitempty"`
	Findings []FindingPayload `json:"findings,omitempty"`
}

type DriftPayload struct {
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
}

type DeployPayload struct {
	URL string `json:"url"`
}

type BusyPayload struct {
	Busy bool `json:"busy"`
}

type wsClient struct {
	conn   net.Conn
	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketRequest(r) {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}

	h, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return
	}

	conn, rw, err := h.Hijack()
	if err != nil {
		http.Error(w, "failed to hijack connection", http.StatusInternalServerError)
		return
	}

	if err := acceptWebSocket(r, rw); err != nil {
		_ = conn.Close()
		return
	}

	client := &wsClient{
		conn:   conn,
		send:   make(chan []byte, 32),
		closed: make(chan struct{}),
	}

	s.registerClient(client)
	go client.writeLoop()
	go client.readLoop(s)
}

func isWebSocketRequest(r *http.Request) bool {
	return hasToken(r.Header.Get("Connection"), "upgrade") &&
		hasToken(r.Header.Get("Upgrade"), "websocket") &&
		r.Header.Get("Sec-WebSocket-Key") != ""
}

func acceptWebSocket(r *http.Request, rw *bufio.ReadWriter) error {
	key := r.Header.Get("Sec-WebSocket-Key")
	sum := sha1.Sum([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	_, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	if err != nil {
		return err
	}
	_, err = rw.WriteString("Upgrade: websocket\r\n")
	if err != nil {
		return err
	}
	_, err = rw.WriteString("Connection: Upgrade\r\n")
	if err != nil {
		return err
	}
	_, err = rw.WriteString(fmt.Sprintf("Sec-WebSocket-Accept: %s\r\n\r\n", accept))
	if err != nil {
		return err
	}
	return rw.Flush()
}

func (c *wsClient) enqueue(data []byte) {
	select {
	case c.send <- data:
	default:
		c.close()
	}
}

func (c *wsClient) writeLoop() {
	for {
		select {
		case data := <-c.send:
			if err := writeTextFrame(c.conn, data); err != nil {
				c.close()
			}
		case <-c.closed:
			return
		}
	}
}

func (c *wsClient) readLoop(s *Server) {
	defer func() {
		s.unregisterClient(c)
		c.close()
	}()
	for {
		opcode, _, err := readFrame(c.conn)
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}
		switch opcode {
		case 0x8:
			return
		case 0x9:
			_ = writeControlFrame(c.conn, 0xA)
		}
	}
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

func writeTextFrame(conn net.Conn, payload []byte) error {
	header := []byte{0x81}
	length := len(payload)
	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127,
			byte(length>>56), byte(length>>48), byte(length>>40), byte(length>>32),
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}

	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func writeControlFrame(conn net.Conn, opcode byte) error {
	_, err := conn.Write([]byte{0x80 | opcode, 0x00})
	return err
}

func readFrame(conn net.Conn) (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := int(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(conn, ext); err != nil {
			return 0, nil, err
		}
		length = int(ext[0])<<8 | int(ext[1])
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(conn, ext); err != nil {
			return 0, nil, err
		}
		length = 0
		for _, b := range ext {
			length = (length << 8) | int(b)
		}
	}

	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(conn, maskKey); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return 0, nil, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}

func hasToken(header, token string) bool {
	for _, part := range strings.Split(strings.ToLower(header), ",") {
		if strings.TrimSpace(part) == token {
			return true
		}
	}
	return false
}
