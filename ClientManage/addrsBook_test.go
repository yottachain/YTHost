package ClientManage

import (
	"fmt"
	"testing"
)

func TestNewAddBookFromServer(t *testing.T) {
	ab, err := NewAddBookFromServer("http://39.105.184.162:8082/active_nodes")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(ab.List())
}
