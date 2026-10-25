package client

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
)

func TestResponseCompatibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		requested, selected string
		ok                  bool
	}{
		{"1.99", "1.0", true},
		{"1.0", "1.0", true},
		{"1.9", "1.10", false},
		{"1.10", "1.9", true},
		{"1.1", "2.0", false},
		{"1.0", "1.01", false},
	} {
		if got := compatible(tc.requested, tc.selected); got != tc.ok {
			t.Errorf("%s against %s=%t", tc.requested, tc.selected, got)
		}
	}
}

func TestMalformedResponses(t *testing.T) {
	t.Parallel()
	for _, frame := range []string{
		`{}`, `{"jsonrpc":"2.0","id":2,"result":{"api_version":"1.0","data":{}}}`,
		`{"jsonrpc":"2.0","id":1,"result":{"api_version":"2.0","data":{}}}`,
		`{"jsonrpc":"2.0","id":1}`, `{"jsonrpc":"2.0","id":1,"error":{},"result":{}}`,
	} {
		var result any
		if err := readResponse(bufio.NewReader(strings.NewReader(frame+"\n")), &result, nil, "1.0"); err == nil {
			t.Fatalf("accepted %s", frame)
		}
	}
}

func TestResponseNotificationsAndErrors(t *testing.T) {
	t.Parallel()
	count := 0
	wire := `{"jsonrpc":"2.0","method":"sync.progress","params":{"kind":"sync.progress"}}` + "\n" +
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"unauthorized","data":{"exit_code":2}}}` + "\n"
	var result any
	err := readResponse(bufio.NewReader(strings.NewReader(wire)), &result, func(api.Notification) { count++ }, "1.0")
	var remote *api.Error
	if !errors.As(err, &remote) || remote.Code != api.Unauthorized || count != 1 {
		t.Fatalf("notifications=%d, error=%v", count, err)
	}
}
