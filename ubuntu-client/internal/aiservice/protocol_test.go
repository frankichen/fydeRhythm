package aiservice

import (
	"bytes"
	"testing"
)

func TestProtocolRoundTrip(t *testing.T) {
	request := []byte("FYDEAI/1 correct 6\n测试")
	action, text, err := readRequest(bytes.NewReader(request))
	if err != nil {
		t.Fatal(err)
	}
	if action != "correct" || text != "测试" {
		t.Fatalf("unexpected request: %q %q", action, text)
	}
	var output bytes.Buffer
	if err := writeResponse(&output, true, "修正"); err != nil {
		t.Fatal(err)
	}
	if output.String() != "OK 6\n修正" {
		t.Fatalf("unexpected response: %q", output.String())
	}
}

func TestRejectOversizedRequest(t *testing.T) {
	_, _, err := readRequest(bytes.NewBufferString("FYDEAI/1 correct 999999\n"))
	if err == nil {
		t.Fatal("expected oversized request error")
	}
}
