package ioStream

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

var (
	Trace   *log.Logger // 记录所有日志
	Info    *log.Logger // 重要的信息
	Warning *log.Logger // 需要注意的信息
	Error   *log.Logger // 非常严重的问题
)

func init() {
	file, err := os.OpenFile("errors.txt",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error log file:", err)
	}

	Trace = log.New(ioutil.Discard,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(file,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(os.Stdout,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(io.MultiWriter(file, os.Stderr),
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

const (
	RES = 'c'
	RPS = 's'
	defaultBufSize = 4096
)

var MAGIC = [3]byte{'Y', 'T', 'A'}

var testCount  = 0

func NewStreamHandler(conn io.ReadWriteCloser, closeRwc bool) (sconn *ReadWriteCloser, cconn *ReadWriteCloser){
	l := sync.Mutex{}

	/*s := NewReadWriter()
	sconn = s
	c := NewReadWriter()
	cconn = c*/

	sconn = NewReadWriter(conn, closeRwc)
	cconn = NewReadWriter(conn, closeRwc)

	buf := bufio.NewWriter(conn)

	testCount ++
	go func(conn io.ReadWriteCloser, sconn *ReadWriteCloser, cconn *ReadWriteCloser) {
		by := make([]byte, 16)
		for {
			if (sconn.GetClose() == true) || (cconn.GetClose() == true) {
				sconn.SetReadErr()
				cconn.SetReadErr()
				_ = sconn.Close()
				_ = cconn.Close()

				return
			}
			f, _, msg, err := DecodeConn(conn, by)

			if err != nil  {
				if err == io.EOF {
					_ = sconn.Close()
					_ = cconn.Close()
				}
				continue
			}
			if f == RES {
				//Error.Printf("request flag is:%s", string(f))
				_ = sconn.ReadAppend(msg)
				//Error.Println("request msg end")
			}else if f == RPS {
				//Error.Printf("response flag is:%s", string(f))
				_ = cconn.ReadAppend(msg)
				//Error.Println("response msg end")
			}else {
				continue
			}
		}
	}(conn, sconn, cconn)

	var WCfunc = func(l *sync.Mutex, conn *ReadWriteCloser, flag byte, buf *bufio.Writer) {
		msg := make([]byte, defaultBufSize + 6)
		for {
			if conn.GetClose() == true {
				return
			}
			n, err:= conn.WriteConsume(defaultBufSize, flag, msg)

			if err != nil {
				//Error.Printf("conn write consume error:%s\n", err)
				continue
			}
			if n > 0 {
				l.Lock()
				_, err = buf.Write(msg[0:n+6])
				if nil == err {
					_ = buf.Flush()
				}
				l.Unlock()
			}
		}
	}

	go WCfunc(&l, sconn, RPS, buf)
	go WCfunc(&l, cconn, RES, buf)

	return
}

func byteToInt16(b []byte) (uint16, error) {
	if len(b) > 2 {
		return 0, errors.New("bytes lenth must less 3\n")
	}
	bytebuff := bytes.NewBuffer(b)
	var ret uint16
	err := binary.Read(bytebuff, binary.BigEndian, &ret)
	if err != nil {
		return 0, err
	}

	return ret, nil
}

func Int16Tobyte(m uint16) ([]byte, error) {

	buf := bytes.NewBuffer([]byte{})
	err := binary.Write(buf, binary.BigEndian, m)

	return buf.Bytes(), err
}

func DecodeConn(conn io.ReadWriteCloser, buf []byte) (flag byte, bLen uint16, msgbuf []byte, err error) {
	//第一个字节是标志 后面三个字节数魔术字 最后两个字节是长度
	n, err := io.ReadFull(conn, buf[0:1])
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return
	}

	b := buf[0]
	if b != RES && b != RPS {
		err = errors.New("conn stream flag error\n")
		return
	}
	flag = b

	//继续三个字节是魔术字
	n, err = io.ReadFull(conn, buf[0:3])
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return
	}

	magic:
	if buf[0] != MAGIC[0] || buf[1] != MAGIC[1] || buf[2] != MAGIC[2] {
		if buf[0] == RES || buf[0] == RPS {
			flag = buf[0]

			buf[0] = buf[1]
			buf[1] = buf[2]
			n, err = io.ReadFull(conn, buf[2:3])
			if err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					err = io.EOF
				}
				return
			}
			goto magic
		}
		err = errors.New("conn stream magic error\n")
		return
	}

	//继续两个字节作为每个消息块的长度
	n, err = io.ReadFull(conn, buf[0:2])
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		return
	}

	bLen, err = byteToInt16(buf[0:2])
	if err == nil {
		msgbuf = make([]byte, bLen)
		n, err = io.ReadFull(conn, msgbuf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
			return
		}
		bLen = uint16(n)
		//Error.Printf("decode conn msg len:%d\n", bLen)
	}

	return
}

type Reader struct {
	buf 		[]byte
	r, w       	int
	l   		sync.Mutex
	rc 			chan bool
}

const (
	ERR_BUFNOZERO = "read buf lenth can't be zero\n"
	INT_MAX = int(^uint(0) >> 1)
)

func (b *Reader) Available() int { return len(b.buf) - b.w }

func (b *Reader) SetReadErr(){
	b.l.Lock()
	defer b.l.Unlock()
	b.rc <- true
	b.r = -1
}

func (b * Reader) Read(p [] byte) (n int, err error){
	n = len(p)
	if n == 0 {
		//Error.Printf("superstratum read error:%s\n", err)
		err = errors.New(ERR_BUFNOZERO)
		return
	}

	<- b.rc

	b.l.Lock()
	defer b.l.Unlock()

	if b.r < 0 {
		err = io.EOF
		return
	}

	// copy as much as we can
	n = copy(p, b.buf[b.r:b.w])
	b.r += n
	if b.Available() <= 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w = b.w - b.r
		b.r = 0
	}

	//Error.Printf("superstratum read length:%d \n", n)

	return n, nil
}

//app data to buf
func (b * Reader) ReadAppend(p [] byte) (err error){
	for {
		b.l.Lock()
		if len(p) > b.Available() {
			copy(b.buf, b.buf[b.r:b.w])
			b.w = b.w - b.r
			b.r = 0
		}
		n := copy(b.buf[b.w:], p)
		b.w += n
		if n == len(p) {
			b.l.Unlock()
			b.rc <- true
			return
		}else {
			p = p[n:]
			b.l.Unlock()
			b.rc <- true
		}
	}
}

func NewReader() *Reader {
	r := new(Reader)
	return r
}

type Writer struct {
	buf 		[]byte
	n       	int
	l   		sync.Mutex
	wc   		chan bool
}

func (b *Writer) Available() int { return len(b.buf) - b.n }

func (b *Writer) Buffered() int { return b.n }

func (b *Writer) Write(p []byte) (nn int, err error) {
	nn = 0
	for  {
		b.l.Lock()
		if len(p) > b.Available() {
			n := copy(b.buf[b.n:], p)
			p = p[n:]
			b.n += n
			nn += n
			b.l.Unlock()
			b.wc <- true
		}else {
			n := copy(b.buf[b.n:], p)
			b.n += n
			nn += n
			b.l.Unlock()
			b.wc <- true
			break
		}
	}

	return nn, nil
}

func (b *Writer) WriteConsume(n int, flag byte, msg []byte) ( nn int, err error){
	//if n <= 0 {
	//	return
	//}

	<- b.wc

	b.l.Lock()
	defer b.l.Unlock()

	if b.Buffered() <= 0 {
		nn = 0
		return
	}

	//前三个字节作为标识
	//msg = make([]byte, n + 3)
	msg[0] = flag
	msg[1] = MAGIC[0]
	msg[2] = MAGIC[1]
	msg[3] = MAGIC[2]
	nn = copy(msg[6:], b.buf[:b.n])
	b.n = b.n - nn
	copy(b.buf, b.buf[nn:])
	//Error.Printf("write consume len:%d\n", nn)
	mLenbyte, err := Int16Tobyte(uint16(nn))
	msg[4] = mLenbyte[0]
	msg[5] = mLenbyte[1]

	//test
	//bLen, _ := byteToInt16(mLenbyte[0:2])
	//Error.Printf("decode write consume len:%d\n", bLen)

	return
}

func NewWriterSize(size int) *Writer {
	if size <= 0 {
		size = defaultBufSize
	}
	return &Writer{
		buf: make([]byte, size),
		n:  0,
		l:   sync.Mutex{},
		wc:  make(chan bool, 128),
	}
}

// NewWriter returns a new Writer whose buffer has the default size.
func NewWriter() *Writer {
	return NewWriterSize(defaultBufSize)
}

type Closer struct {
	rwc    io.ReadWriteCloser
	isClose 	bool
	isCloseRwc 	bool
}

func (c * Closer) Close() error{
	c.isClose = true

	if c.isCloseRwc == true {
		//Error.Println("iostream closed begin")
		err := c.rwc.Close()
		if err == nil {
			c.isCloseRwc = false
		}
		return err
	}else {
		return nil
	}
}

func (c * Closer) GetClose() bool {
	return c.isClose
}

type ReadWriteCloser struct {
	*Closer
	*Reader
	*Writer
}

func NewReadWriter(iorwc io.ReadWriteCloser, iscolseRwc bool) *ReadWriteCloser {
	r := new(Reader)
	r.buf = make([]byte, defaultBufSize)
	r.r = 0
	r.w = 0
	r.l = sync.Mutex{}
	r.rc =  make(chan bool, 128)
	/*r := &Reader{
		buf: make([]byte, defaultBufSize),
		r:   0,
		w:   0,
		l:   sync.Mutex{},
		rc:	 make(chan bool, 128),
	}*/
	w := new(Writer)
	w.buf = make([]byte, defaultBufSize)
	w.n = 0
	w.l = sync.Mutex{}
	w.wc =  make(chan bool, 128)
	/*w := &Writer{
		buf: make([]byte, defaultBufSize),
		n:   0,
		l:   sync.Mutex{},
		wc:  make(chan bool, 128),
	}*/
	c := new(Closer)
	c.isClose = false
	c.rwc = iorwc
	c.isCloseRwc = iscolseRwc
	//c := &Closer{false}
	rwc := new(ReadWriteCloser)
	rwc.Reader = r
	rwc.Writer = w
	rwc.Closer = c
	//return &ReadWriteCloser{c, r, w }
	return rwc
}