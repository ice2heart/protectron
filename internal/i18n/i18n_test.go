package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemplates(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"ru.json": `{"pong_msg": "живой", "join_msg": "Привет, ${user_title}!", "ru_only": "только ру"}`,
		"en.json": `{"pong_msg": "alive", "join_msg": "Hi, ${user_title}!"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSubstitution(t *testing.T) {
	b, err := Load(writeTemplates(t))
	if err != nil {
		t.Fatal(err)
	}
	got := b.T("en", "join_msg", map[string]string{"user_title": "@user"})
	if got != "Hi, @user!" {
		t.Errorf("got %q", got)
	}
	got = b.T("ru", "join_msg", map[string]string{"user_title": "@юзер"})
	if got != "Привет, @юзер!" {
		t.Errorf("got %q", got)
	}
}

func TestFallbacks(t *testing.T) {
	b, err := Load(writeTemplates(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.T("de", "pong_msg", nil); got != "живой" {
		t.Errorf("unknown lang: got %q, want ru fallback", got)
	}
	if got := b.T("en", "ru_only", nil); got != "только ру" {
		t.Errorf("missing key: got %q, want ru fallback", got)
	}
	if got := b.T("ru", "no_such_key", nil); got != "" {
		t.Errorf("missing everywhere: got %q, want empty", got)
	}
}

func TestMissingParamRendersEmpty(t *testing.T) {
	b, err := Load(writeTemplates(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.T("en", "join_msg", nil); got != "Hi, !" {
		t.Errorf("got %q", got)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("empty dir: want error")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"b"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("missing ru.json: want error")
	}
}

func TestRealTemplates(t *testing.T) {
	b, err := Load("../../templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"ru"} {
		if got := b.T(lang, "join_msg", map[string]string{"user_title": "@user"}); got == "" {
			t.Errorf("%s join_msg rendered empty", lang)
		}
	}
}
