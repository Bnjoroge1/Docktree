package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestRendererJSON(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{JSON: true, Writer: &buf}
	r.Render(map[string]string{"ok": "yes"}, func(_ io.Writer, _ any) {})
	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != "yes" {
		t.Fatalf("bad json output: %#v", got)
	}
}

func TestRendererHuman(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{Writer: &buf, IsTTY: true}
	r.Render("value", func(w io.Writer, data any) {
		_, _ = w.Write([]byte("human:" + data.(string)))
	})
	if buf.String() != "human:value" {
		t.Fatalf("bad human output: %q", buf.String())
	}
}

func TestRendererNonTTYErrorIsStructured(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{Writer: &buf}
	r.Error("config", "bad config", nil)
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("non-tty error should be json: %q", buf.String())
	}
}
