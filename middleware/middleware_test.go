package middleware

import "testing"

type testMw struct {
}

func (mw testMw)Write([]byte)[]byte{
	return nil
}

func TestMiddlewareMngr_Use(t *testing.T) {
	//mwm:=New()
	//mwm.Use()
}
