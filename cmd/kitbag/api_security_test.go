//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/api"
)

func TestAPISecurity(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	for name, tc := range map[string]struct {
		request []byte
		code    int
	}{
		"missing-token":  {[]byte(`{"jsonrpc":"2.0","id":1,"method":"version.get","params":{"api_version":"1.0"}}`), api.Unauthorized},
		"wrong-token":    {apiRequest(t, "version.get", `{"token":"secret-canary"}`, fixture.token), api.Unauthorized},
		"bad-version":    {apiRequest(t, "version.get", `{"api_version":"2.0"}`, fixture.token), api.UnsupportedVersion},
		"malformed":      {[]byte(`{"jsonrpc":`), api.ParseError},
		"batch":          {[]byte(`[]`), api.InvalidRequest},
		"invalid-id":     {[]byte(`{"jsonrpc":"2.0","id":{},"method":"version.get","params":{}}`), api.InvalidRequest},
		"invalid-params": {[]byte(`{"jsonrpc":"2.0","id":1,"method":"version.get","params":null}`), api.InvalidParams},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := fixture.exchange(t, fixture.connect(t), tc.request)
			if response.Error == nil || response.Error.Code != tc.code {
				t.Fatalf("response: %+v", response)
			}
			data, err := json.Marshal(response.Error)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(fixture.token)) || bytes.Contains(data, []byte("secret-canary")) {
				t.Fatalf("error disclosed secret: %s", data)
			}
		})
	}
}

func TestAPIRequestLimit(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	response := fixture.exchange(t, fixture.connect(t), []byte(strings.Repeat("x", api.MaxRequestBytes)))
	if response.Error == nil || response.Error.Code != api.RequestTooLarge {
		t.Fatalf("oversized request: %+v", response)
	}
	// An oversized frame cannot stop other clients.
	response = fixture.exchange(t, fixture.connect(t), apiRequest(t, "version.get", "{}", fixture.token))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
}

func TestAPIEventsAndProgress(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	subscriber := fixture.connect(t)
	subscribed := fixture.exchange(t, subscriber, apiRequest(t, "events.subscribe", "{}", fixture.token))
	if subscribed.Error != nil {
		t.Fatal(subscribed.Error)
	}
	for _, kind := range []string{"drift.detected", "update.available"} {
		fixture.events.Publish(api.Event{Kind: kind})
		assertAPINotification(t, json.NewDecoder(subscriber), kind)
	}
	caller := fixture.connect(t)
	if _, err := caller.Write(append(apiRequest(t, "sync.run", `{"no_pull":true}`, fixture.token), '\n')); err != nil {
		t.Fatal(err)
	}
	assertAPIProgress(t, json.NewDecoder(caller))
	decoder := json.NewDecoder(subscriber)
	assertAPINotification(t, decoder, "sync.started")
	assertAPINotification(t, decoder, "sync.finished")
}

func assertAPINotification(t *testing.T, decoder *json.Decoder, kind string) {
	t.Helper()
	var frame json.RawMessage
	if err := decoder.Decode(&frame); err != nil {
		t.Fatal(err)
	}
	validateAPIValue(t, "notification", frame, true)
	var notification api.Notification
	if err := json.Unmarshal(frame, &notification); err != nil {
		t.Fatal(err)
	}
	if notification.Method != kind || notification.Params.Kind != kind {
		t.Fatalf("notification=%s", frame)
	}
}

func assertAPIProgress(t *testing.T, decoder *json.Decoder) {
	t.Helper()
	phases := []string{}
	for {
		var message struct {
			Error  *api.Error  `json:"error"`
			Result *api.Result `json:"result"`
			Method string      `json:"method"`
			Params api.Event   `json:"params"`
		}
		if err := decoder.Decode(&message); err != nil {
			t.Fatal(err)
		}
		if message.Method == "sync.progress" {
			phases = append(phases, message.Params.Phase)
			continue
		}
		if message.Error != nil || message.Result == nil {
			t.Fatalf("sync response: %+v", message)
		}
		break
	}
	if strings.Join(phases, ",") != "started,plan,apply,includes,finished" {
		t.Fatalf("phases=%v", phases)
	}
}

func TestAPISlowSubscriberIsolation(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	slow := fixture.connect(t)
	if response := fixture.exchange(t, slow, apiRequest(t, "events.subscribe", "{}", fixture.token)); response.Error != nil {
		t.Fatal(response.Error)
	}
	// Stop reading this subscriber, then publish far beyond both queue bounds.
	// An unrelated request must still finish within its connection deadline.
	const floodMultiplier = 1024
	for range api.EventBuffer * floodMultiplier {
		fixture.events.Publish(api.Event{Kind: "drift.detected"})
	}
	response := fixture.exchange(t, fixture.connect(t), apiRequest(t, "version.get", "{}", fixture.token))
	if response.Error != nil || response.Result == nil {
		t.Fatalf("slow subscriber blocked daemon: %+v", response)
	}
}
