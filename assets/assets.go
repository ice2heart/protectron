// Package assets embeds binary assets into the bot binary.
package assets

import _ "embed"

// CaptchaFont is DejaVu Sans Bold (see DEJAVU-LICENSE): TTF-flavored and
// Cyrillic-capable, both required by the freetype-based captcha renderer.
//
//go:embed DejaVuSans-Bold.ttf
var CaptchaFont []byte
