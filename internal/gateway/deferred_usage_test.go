package gateway

import (
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// The Cursor plugin's step that collected an earlier step's late usage
// event (yetone/magpie#1053) tells it apart, as magpie_deferred: the
// step's prompt_tokens stay its own, which a client reads as how full its
// context is (pi read 366940 = 181270 + 185670 of a 300k window, where
// 185670 was used), and magpie bills both.
const deferredJSON = `{"id":"c","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":185670,"completion_tokens":2240,"total_tokens":187910,"prompt_tokens_details":{"cached_tokens":181266,"cache_write_tokens":4400},"magpie_deferred":{"prompt_tokens":181270,"completion_tokens":1913,"cached_tokens":177533,"cache_write_tokens":3735}}}`

var deferredStream = "data: " + deferredJSON + "\n\ndata: [DONE]\n\n"

func TestSniffChatDeferredUsage(t *testing.T) {
	s := newSniffer(provider.Chat, "text/event-stream")
	s.write([]byte(deferredStream))
	got := s.usage()
	if d := got.Deferred; d == nil || *d != (Usage{Input: 2, Output: 1913, CacheRead: 177533, CacheWrite: 3735}) {
		t.Fatalf("deferred %+v", got.Deferred)
	}
	got.Deferred = nil
	if own := (Usage{Input: 4, Output: 2240, CacheRead: 181266, CacheWrite: 4400, ResponseID: "c", Stop: "tool_calls"}); got != own {
		t.Fatalf("own %+v want %+v", got, own)
	}
}

func TestDeferredUsageIsBilledNotPrompt(t *testing.T) {
	s := newSniffer(provider.Chat, "text/event-stream")
	s.write([]byte(deferredStream))
	u := s.usage()
	b := u.billed()
	if b.Deferred != nil || b.Input != 6 || b.Output != 2240+1913 || b.CacheRead != 181266+177533 || b.CacheWrite != 4400+3735 {
		t.Fatalf("billed %+v", b)
	}
	if u.prompt() != 185670 {
		t.Fatalf("the request's own prompt %d", u.prompt())
	}
	rs := routeUsage("cursor", "claude-opus-5-5", u)
	if len(rs) != 2 {
		t.Fatalf("tiers %+v", rs)
	}
	// the route's prompt, as its context is read, is the request's own
	if n := promptCounted(rs); n != 185670 {
		t.Fatalf("promptCounted %d", n)
	}
	var in, out int
	for _, r := range rs {
		in += r.Input + r.CacheRead + r.CacheWrite
		out += r.Output
	}
	if in != 185670+181270 || out != 2240+1913 {
		t.Fatalf("priced %d in %d out", in, out)
	}
}

func TestChatDecoderKeepsDeferredUsage(t *testing.T) {
	d := &chatDecoder{}
	var u Usage
	for _, line := range []string{`{"id":"c","choices":[{"index":0,"delta":{"content":""}}]}`, deferredJSON} {
		if err := d.decode(line, func(ev Event) {
			if ev.Kind == KUsage {
				u.add(ev.Usage)
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
	if u.prompt() != 185670 || u.Deferred == nil || u.Deferred.prompt() != 181270 {
		t.Fatalf("usage %+v deferred %+v", u, u.Deferred)
	}
	// a client of another API is told the request's own, as before
	if c := u.chat(); c["prompt_tokens"] != 185670 {
		t.Fatalf("told %v", c)
	}
}

func TestNoDeferredUsageAsBefore(t *testing.T) {
	s := newSniffer(provider.Chat, "text/event-stream")
	s.write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":60}}}\n\n"))
	u := s.usage()
	if u.Deferred != nil || u.billed() != u || len(routeUsage("p", "m", u)) != 1 {
		t.Fatalf("%+v", u)
	}
}

// a real step of the Cursor plugin, its tool steps' events collected late
// (STEP_WAIT shortened): its own prompt 1371, and 2619 of earlier steps
const realDeferredChunk = `{"id":"chatcmpl-71dd4976e7f654f774cd7619","object":"chat.completion.chunk","created":1791866687,"model":"composer-2.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1371,"completion_tokens":48,"total_tokens":1419,"prompt_tokens_details":{"cached_tokens":1328,"cache_write_tokens":0},"magpie_deferred":{"prompt_tokens":2619,"completion_tokens":106,"cached_tokens":2533,"cache_write_tokens":0}}}`

// Claude Code on Cursor: the chat reply is translated, and Claude Code is
// told the step's own prompt, while magpie bills the earlier steps' too
func TestTranslatedReplyTellsOwnUsageBillsDeferred(t *testing.T) {
	d := &chatDecoder{}
	var c collector
	if err := d.decode(realDeferredChunk, c.add); err != nil {
		t.Fatal(err)
	}
	res := c.finish()
	if res.Usage.prompt() != 1371 || res.Usage.Deferred == nil || res.Usage.Deferred.prompt() != 2619 || res.Usage.Deferred.Output != 106 {
		t.Fatalf("usage %+v deferred %+v", res.Usage, res.Usage.Deferred)
	}
	a := res.Usage.anthropic()
	if a.InputTokens+a.CacheReadInputTokens+a.CacheCreationInputTokens != 1371 || a.OutputTokens != 48 {
		t.Fatalf("told Claude Code %+v", a)
	}
	if b := res.Usage.billed(); b.prompt() != 1371+2619 || b.Output != 48+106 {
		t.Fatalf("billed %+v", b)
	}
}

// a step with nothing of its own and an earlier step's usage still tells
// the gateway (chatDecoder gave up on a usage of 0 prompt and 0 completion)
func TestChatDecoderKeepsDeferredUsageWithNoOwn(t *testing.T) {
	d := &chatDecoder{}
	var got []Usage
	line := `{"id":"c","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"magpie_deferred":{"prompt_tokens":1148,"completion_tokens":50,"cached_tokens":1100,"cache_write_tokens":0}}}`
	if err := d.decode(line, func(ev Event) {
		if ev.Kind == KUsage {
			got = append(got, ev.Usage)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].prompt() != 0 || got[0].Deferred == nil || got[0].Deferred.prompt() != 1148 || got[0].Deferred.Output != 50 {
		t.Fatalf("usage %+v", got)
	}
}
