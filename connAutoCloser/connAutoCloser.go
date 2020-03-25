package connAutoCloser

import (
	"net"
	"time"
)

type ConnAutoCloser struct {
	net.Conn
	outtime time.Duration
	timer   *time.Timer
}

func New(conn net.Conn) *ConnAutoCloser {
	t := time.NewTimer(time.Minute)
	go func() {
		t.Stop()
		<-t.C
		conn.Close()
	}()
	return &ConnAutoCloser{conn, time.Minute, t}
}

func (conn *ConnAutoCloser) Read(buf []byte) (int, error) {
	n, err := conn.Conn.Read(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		_ = conn.timer.Reset(conn.outtime)
	}
	return n, err
}

func (conn *ConnAutoCloser) Write(buf []byte) (int, error) {
	n, err := conn.Conn.Write(buf)
	if err != nil {
		return n, err
	}
	if n > 0 {
		_ = conn.timer.Reset(conn.outtime)
	}
	return n, err
}

func (conn *ConnAutoCloser) SetOuttime(duration time.Duration) {
	conn.outtime = duration
	conn.timer.Reset(conn.outtime)
}
