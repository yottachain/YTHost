package connAutoCloser

import (
	"net"
	"time"
)

type ConnAutoCloser struct {
	net.Conn
	outtime time.Duration
	timer   *time.Timer
	c       chan struct{}
}

func New(conn net.Conn, otime time.Duration) *ConnAutoCloser {
	t := time.NewTimer(otime)
	cclose := &ConnAutoCloser{conn, otime, t, make(chan struct{}, 1)}
	go func() {
		<-t.C
		if conn != nil {
			conn.Close()
		}
	}()
	return cclose
}

func (conn *ConnAutoCloser) Stop() {
	conn.timer.Reset(0)
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
	select {
	case conn.c <- struct{}{}:
		conn.timer.Reset(conn.outtime)
		<-conn.c
	default:
		return
	}
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
