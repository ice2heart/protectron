// Package i18n renders message templates from templates/<lang>.json.
// Templates keep the Mako-style ${var} placeholders of the Python bot;
// substitution is done with an os.Expand mapper, so the JSON files need
// no conversion.
package i18n

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const FallbackLang = "ru"

type Bundle struct {
	langs map[string]map[string]string
}

// Load reads every <lang>.json in dir. The file base name is the language code.
func Load(dir string) (*Bundle, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("i18n: no templates found in %q", dir)
	}

	b := &Bundle{langs: make(map[string]map[string]string)}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var msgs map[string]string
		if err := json.Unmarshal(raw, &msgs); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", p, err)
		}
		lang := strings.TrimSuffix(filepath.Base(p), ".json")
		b.langs[lang] = msgs
	}

	if _, ok := b.langs[FallbackLang]; !ok {
		return nil, fmt.Errorf("i18n: fallback language %q missing in %q", FallbackLang, dir)
	}
	return b, nil
}

// Langs reports the loaded language codes.
func (b *Bundle) Langs() []string {
	out := make([]string, 0, len(b.langs))
	for l := range b.langs {
		out = append(out, l)
	}
	return out
}

// Has reports whether lang is loaded.
func (b *Bundle) Has(lang string) bool {
	_, ok := b.langs[lang]
	return ok
}

// T renders the template key in lang, substituting ${var} from params.
// Unknown languages fall back to FallbackLang; a missing key is logged
// and rendered as an empty string, matching the old bot's behavior.
func (b *Bundle) T(lang, key string, params map[string]string) string {
	msgs, ok := b.langs[lang]
	if !ok {
		msgs = b.langs[FallbackLang]
	}
	tmpl, ok := msgs[key]
	if !ok {
		if fb, fbOK := b.langs[FallbackLang][key]; fbOK {
			tmpl = fb
		} else {
			slog.Error("missing template key", "lang", lang, "key", key)
			return ""
		}
	}
	return os.Expand(tmpl, func(name string) string {
		if v, ok := params[name]; ok {
			return v
		}
		slog.Error("missing template param", "lang", lang, "key", key, "param", name)
		return ""
	})
}

// Expand substitutes ${var} placeholders from params in an arbitrary string,
// e.g. an admin-supplied greeting. Unknown placeholders render as empty.
func Expand(text string, params map[string]string) string {
	return ExpandFunc(text, params, nil)
}

// ExpandFunc is Expand with a hook applied to the literal spans only, leaving
// substituted values untouched. Callers that must escape admin-supplied text
// need this: escaping the whole string first would mangle the ${var} syntax
// itself (e.g. Markdown turns ${user_title} into ${user\_title}, which no
// longer matches any param), so the escaping has to skip the placeholders.
func ExpandFunc(text string, params map[string]string, literal func(string) string) string {
	if literal == nil {
		literal = func(s string) string { return s }
	}
	var b strings.Builder
	rest := text
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "}")
		if j < 0 {
			break
		}
		name := rest[i+2 : i+j]
		b.WriteString(literal(rest[:i]))
		b.WriteString(params[name])
		rest = rest[i+j+1:]
	}
	b.WriteString(literal(rest))
	return b.String()
}
