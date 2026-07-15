package captcha

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	c, err := Generate("ru", 8)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Answer) != 8 {
		t.Fatalf("answer length: %d", len(c.Answer))
	}
	charset := string(Charset("ru"))
	for _, ch := range c.Answer {
		if !strings.Contains(charset, ch) {
			t.Errorf("answer char %q not in ru charset", ch)
		}
	}

	// One token per answer position, even with duplicate chars.
	if len(c.Buttons) != 8 {
		t.Fatalf("buttons: %d", len(c.Buttons))
	}
	fromButtons := map[string]int{}
	for token, ch := range c.Buttons {
		if len(token) != 8 {
			t.Errorf("token %q: want 8 hex chars", token)
		}
		fromButtons[ch]++
	}
	fromAnswer := map[string]int{}
	for _, ch := range c.Answer {
		fromAnswer[ch]++
	}
	for ch, n := range fromAnswer {
		if fromButtons[ch] != n {
			t.Errorf("char %q: %d buttons, %d answer occurrences", ch, fromButtons[ch], n)
		}
	}

	img, err := png.Decode(bytes.NewReader(c.Image))
	if err != nil {
		t.Fatalf("image is not a valid PNG: %v", err)
	}
	if img.Bounds().Dx() != imgWidth || img.Bounds().Dy() != imgHeight {
		t.Errorf("image size: %v", img.Bounds())
	}
}

func TestKeyboardShape(t *testing.T) {
	c, err := Generate("ru", 8)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if len(sid) != 16 {
		t.Fatalf("session id %q: want 16 hex chars", sid)
	}

	kb := c.Keyboard(sid)
	rows := kb.InlineKeyboard
	if len(rows) != 3 {
		t.Fatalf("rows: %d", len(rows))
	}
	if len(rows[0]) != 4 || len(rows[1]) != 4 || len(rows[2]) != 1 {
		t.Fatalf("row sizes: %d/%d/%d", len(rows[0]), len(rows[1]), len(rows[2]))
	}
	if rows[2][0].Text != "⌫" || rows[2][0].CallbackData != BackspaceData(sid) {
		t.Errorf("backspace button: %+v", rows[2][0])
	}

	seen := map[string]bool{}
	for _, row := range rows[:2] {
		for _, btn := range row {
			if len(btn.CallbackData) >= 64 {
				t.Errorf("callback data %q: %d bytes, exceeds telegram limit", btn.CallbackData, len(btn.CallbackData))
			}
			gotSID, token, ok := ParseCallbackData(btn.CallbackData)
			if !ok || gotSID != sid {
				t.Fatalf("bad callback data %q", btn.CallbackData)
			}
			if IsBackspace(token) {
				t.Errorf("char button parsed as backspace: %q", btn.CallbackData)
			}
			if c.Buttons[token] != btn.Text {
				t.Errorf("token %q maps to %q, button shows %q", token, c.Buttons[token], btn.Text)
			}
			if seen[token] {
				t.Errorf("token %q reused", token)
			}
			seen[token] = true
		}
	}
}

func TestKeyboardOddLength(t *testing.T) {
	c, err := Generate("en", 5)
	if err != nil {
		t.Fatal(err)
	}
	kb := c.Keyboard("0123456789abcdef")
	rows := kb.InlineKeyboard
	if len(rows) != 3 || len(rows[0]) != 3 || len(rows[1]) != 2 {
		t.Fatalf("odd split wrong: %d rows, %d/%d", len(rows), len(rows[0]), len(rows[1]))
	}
}

func TestParseCallbackData(t *testing.T) {
	sid := "0123456789abcdef"
	for _, tc := range []struct {
		data      string
		wantOK    bool
		wantSID   string
		wantToken string
	}{
		{CallbackData(sid, "a1b2c3d4"), true, sid, "a1b2c3d4"},
		{BackspaceData(sid), true, sid, "bs"},
		{"btn_a", false, "", ""},
		{"c:", false, "", ""},
		{"c::", false, "", ""},
		{"c:abc:", false, "", ""},
		{"c::tok", false, "", ""},
		{"", false, "", ""},
		{"x:0123456789abcdef:a1b2c3d4", false, "", ""},
	} {
		gotSID, gotToken, ok := ParseCallbackData(tc.data)
		if ok != tc.wantOK || gotSID != tc.wantSID || gotToken != tc.wantToken {
			t.Errorf("ParseCallbackData(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.data, gotSID, gotToken, ok, tc.wantSID, tc.wantToken, tc.wantOK)
		}
	}
}

func TestCharsetFallback(t *testing.T) {
	if string(Charset("de")) != string(Charset("ru")) {
		t.Error("unknown lang should fall back to ru charset")
	}
	for lang, cs := range charsets {
		seen := map[rune]bool{}
		for _, r := range cs {
			if seen[r] {
				t.Errorf("%s charset: duplicate rune %q", lang, r)
			}
			seen[r] = true
		}
	}
}
