// Package captcha generates image captchas with opaque button tokens.
//
// All randomness (text, tokens, session ids, layout shuffle) comes from
// crypto/rand: the callback data must not be predictable, that is the whole
// point of the token scheme.
package captcha

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/go-telegram/bot/models"
)

// Charsets per language, confusables excluded:
//   - ru drops а/е/о/с/х/у (Latin lookalikes), soft/hard signs, and 0/1
//   - en drops i/l/o/j/y and 0/1/9
var charsets = map[string][]rune{
	"ru": []rune("бвгджзиклмнпрстуфцчшщэюя23456789"),
	"en": []rune("asdfghkzxcvbnmqwertu2345678"),
}

// Charset returns the alphabet for lang, falling back to ru.
func Charset(lang string) []rune {
	if cs, ok := charsets[lang]; ok {
		return cs
	}
	return charsets["ru"]
}

// Button is one keyboard button: a visible char and its opaque token.
type Button struct {
	Token string
	Char  string
}

// Captcha is one generated challenge.
type Captcha struct {
	// Answer is the expected press sequence (chars may repeat).
	Answer []string
	// Buttons maps opaque token -> char; stored in the session document,
	// never exposed in the keyboard markup.
	Buttons map[string]string
	// Image is the rendered PNG.
	Image []byte

	// layout is the shuffled button order for the keyboard.
	layout []Button
}

// Generate creates a challenge of length chars from lang's charset.
func Generate(lang string, length int) (*Captcha, error) {
	charset := Charset(lang)

	answer := make([]string, length)
	runes := make([]rune, length)
	for i := range answer {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return nil, err
		}
		runes[i] = charset[n.Int64()]
		answer[i] = string(charset[n.Int64()])
	}

	buttons := make(map[string]string, length)
	layout := make([]Button, 0, length)
	for _, char := range answer {
		token, err := newToken(buttons)
		if err != nil {
			return nil, err
		}
		buttons[token] = char
		layout = append(layout, Button{Token: token, Char: char})
	}
	if err := shuffle(layout); err != nil {
		return nil, err
	}

	img, err := render(string(runes))
	if err != nil {
		return nil, err
	}

	return &Captcha{
		Answer:  answer,
		Buttons: buttons,
		Image:   img,
		layout:  layout,
	}, nil
}

// NewSessionID returns a 16-hex-char random id (also the Mongo _id).
func NewSessionID() (string, error) {
	return randomHex(8)
}

const backspaceToken = "bs"

// CallbackData builds the callback payload: c:<session_id>:<token>.
func CallbackData(sessionID, token string) string {
	return fmt.Sprintf("c:%s:%s", sessionID, token)
}

// BackspaceData builds the backspace payload: c:<session_id>:bs.
func BackspaceData(sessionID string) string {
	return CallbackData(sessionID, backspaceToken)
}

// ParseCallbackData splits a payload produced by CallbackData.
func ParseCallbackData(data string) (sessionID, token string, ok bool) {
	if len(data) < 4 || data[:2] != "c:" {
		return "", "", false
	}
	rest := data[2:]
	for i := 0; i < len(rest); i++ {
		if rest[i] == ':' {
			sessionID, token = rest[:i], rest[i+1:]
			if sessionID == "" || token == "" {
				return "", "", false
			}
			return sessionID, token, true
		}
	}
	return "", "", false
}

// IsBackspace reports whether a parsed token is the backspace token.
func IsBackspace(token string) bool {
	return token == backspaceToken
}

// Keyboard renders the shuffled buttons as 2 rows of len/2, plus a ⌫ row.
func (c *Captcha) Keyboard(sessionID string) *models.InlineKeyboardMarkup {
	half := (len(c.layout) + 1) / 2
	rows := make([][]models.InlineKeyboardButton, 0, 3)
	for _, chunk := range [][]Button{c.layout[:half], c.layout[half:]} {
		row := make([]models.InlineKeyboardButton, 0, len(chunk))
		for _, b := range chunk {
			row = append(row, models.InlineKeyboardButton{
				Text:         b.Char,
				CallbackData: CallbackData(sessionID, b.Token),
			})
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "⌫", CallbackData: BackspaceData(sessionID)},
	})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// newToken returns an 8-hex token not yet present in taken.
func newToken(taken map[string]string) (string, error) {
	for {
		t, err := randomHex(4)
		if err != nil {
			return "", err
		}
		if _, dup := taken[t]; !dup {
			return t, nil
		}
	}
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// shuffle is a crypto/rand Fisher–Yates.
func shuffle(items []Button) error {
	for i := len(items) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := n.Int64()
		items[i], items[j] = items[j], items[i]
	}
	return nil
}
