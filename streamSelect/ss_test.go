package streamSelect

import (
	"bytes"
	"testing"
)

func TestBuf(t *testing.T) {
	buf := bytes.NewBuffer([]byte{})
	buf.Write([]byte{1, 2, 3})

	b := make([]byte, 2)

	n, err := buf.Read(b)
	t.Log(n, err)

	n, err = buf.Read(b)
	t.Log(n, err)
	n, err = buf.Read(b)
	t.Log(n, err)
	buf.Write([]byte{1, 2, 3})
	n, err = buf.Read(b)
	t.Log(n, err)
}
