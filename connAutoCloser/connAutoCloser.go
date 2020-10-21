package connAutoCloser

import (
	"net"
	"time"
)

type ConnAutoCloser struct {
	net.Conn
}

func New(conn net.Conn) *ConnAutoCloser {
	conn.SetDeadline(time.Now().Add(time.Second * 30))
	return &ConnAutoCloser{conn}
}

func (conn *ConnAutoCloser) Read(buf []byte) (int, error) {
	n, err := conn.Conn.Read(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		conn.ResetTimer()
	}
	return n, err
}

func (conn *ConnAutoCloser) ResetTimer() {
	conn.Conn.SetDeadline(time.Now().Add(time.Second * 30))
}

func (conn *ConnAutoCloser) Write(buf []byte) (int, error) {
	n, err := conn.Conn.Write(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		conn.ResetTimer()
	}
	return n, err
}
