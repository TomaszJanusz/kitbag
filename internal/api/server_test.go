package api

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestOversizedRequestDrainsTail(t *testing.T) {
	t.Parallel()
	for name, tail := range map[string]string{
		"newline":      "\n",
		"bounded-tail": strings.Repeat("x", MaxRequestBytes-1) + "\n",
		"no-newline":   strings.Repeat("x", MaxRequestBytes),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server, client := net.Pipe()
			t.Cleanup(func() { closeQuiet(client) })
			if err := client.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				serveConnection(t.Context(), server, Handlers{})
			}()
			// Split at the limit so closing before consuming the tail always
			// fails, even on systems that buffer a whole oversized socket write.
			if _, err := io.WriteString(client, strings.Repeat("x", MaxRequestBytes)); err != nil {
				t.Fatal(err)
			}
			var response Response
			if err := json.NewDecoder(client).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response.Error == nil || response.Error.Code != RequestTooLarge {
				t.Fatalf("oversized request: %+v", response)
			}
			if _, err := io.WriteString(client, tail); err != nil {
				t.Fatalf("server closed before draining tail: %v", err)
			}
			if data, err := io.ReadAll(client); err != nil || len(data) != 0 {
				t.Fatalf("connection did not close cleanly: %q, %v", data, err)
			}
			<-done
		})
	}
}
