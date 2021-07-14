package cap

import (
	"io"

	"github.com/dchest/captcha"
)

//GetNewCaptcha placeholed
func GetNewCaptcha(img io.Writer) string {
	id := captcha.NewLen(6)
	captcha.WriteImage(img, id, 300, 150)
	return id
}

//CheckCaptcha placeholed
func CheckCaptcha(id string, line string) (result bool) {
	result = captcha.VerifyString(id, line)
	return
}
