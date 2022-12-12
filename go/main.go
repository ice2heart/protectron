package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	cap "github.com/ice2heart/protectron/captcha"
	"github.com/joho/godotenv"
	tb "gopkg.in/tucnak/telebot.v2"
)

type caps struct {
	userInput        string
	captchaId        string
	userID           int64
	chatID           int64
	captchaMessageID string
	loginMessageID   string
	success          bool
}

var (
	runningCaps map[string]*caps
	b           *tb.Bot
)

func clean(id string) {
	item := runningCaps[id]
	log.Println("Input clean")
	// delete captcha
	err := b.Delete(&tb.StoredMessage{MessageID: item.captchaMessageID, ChatID: item.chatID})
	if err != nil {
		log.Println("opsy")
	}
	// delete join message
	err = b.Delete(&tb.StoredMessage{MessageID: item.loginMessageID, ChatID: item.chatID})
	if err != nil {
		log.Println("opsy")
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if v := recover(); v != nil {
			cancel()
		}
	}()
	// Catch signals and close listener socket
	intCh := make(chan os.Signal, 1)
	signal.Notify(intCh, os.Interrupt, syscall.SIGTERM)

	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	token := os.Getenv("API_TOKEN")
	b, err = tb.NewBot(tb.Settings{
		Token:  token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	runningCaps = make(map[string]*caps)

	b.Handle("/hello", func(m *tb.Message) {
		var (
			menu     = &tb.ReplyMarkup{ResizeReplyKeyboard: true}
			btn1     = menu.Data("1", "1", "1")
			btn2     = menu.Data("2", "2", "2")
			btn3     = menu.Data("3", "3", "3")
			btn4     = menu.Data("4", "4", "4")
			btn5     = menu.Data("5", "5", "5")
			btn6     = menu.Data("6", "6", "6")
			btn7     = menu.Data("7", "7", "7")
			btn8     = menu.Data("8", "8", "8")
			btn9     = menu.Data("9", "9", "9")
			btn0     = menu.Data("0", "0", "0")
			btnBkspc = menu.Data("|<=", "bkspc")
		)

		messageIdOrig, chatID := m.MessageSig()
		messageId := messageIdOrig + strconv.FormatInt(chatID, 10)
		log.Println(chatID, m.Sender.ID)

		menu.Inline(
			menu.Row(btn7, btn8, btn9),
			menu.Row(btn4, btn5, btn6),
			menu.Row(btn1, btn2, btn3),
			menu.Row(btn0, btnBkspc),
		)
		btnFunc := func(c *tb.Callback) {
			messageId, chatID := c.Message.ReplyTo.MessageSig()
			messageId += strconv.FormatInt(chatID, 10)
			log.Println(c.Data, chatID, c.Sender.ID)
			if _, ok := runningCaps[messageId]; !ok {
				log.Println("session is not found")
				return
			}
			if c.Sender.ID != runningCaps[messageId].userID {
				b.Respond(c, &tb.CallbackResponse{Text: "Not for you"})
				return
			}
			runningCaps[messageId].userInput += c.Data
			if len(runningCaps[messageId].userInput) == 6 {
				res := cap.CheckCaptcha(runningCaps[messageId].captchaId, runningCaps[messageId].userInput)
				b.Respond(c, &tb.CallbackResponse{Text: fmt.Sprintf("You are %v", res)})
				clean(messageId)

				runningCaps[messageId].success = res
				return
			}
			b.Respond(c, &tb.CallbackResponse{Text: "Input: " + runningCaps[messageId].userInput})
		}
		b.Handle(&btn1, btnFunc)
		b.Handle(&btn2, btnFunc)
		b.Handle(&btn3, btnFunc)
		b.Handle(&btn4, btnFunc)
		b.Handle(&btn5, btnFunc)
		b.Handle(&btn6, btnFunc)
		b.Handle(&btn7, btnFunc)
		b.Handle(&btn8, btnFunc)
		b.Handle(&btn9, btnFunc)
		b.Handle(&btn0, btnFunc)
		b.Handle(&btnBkspc, func(c *tb.Callback) {
			messageId, chatID := c.Message.ReplyTo.MessageSig()
			messageId += strconv.FormatInt(chatID, 10)
			log.Println(c.Data, chatID, c.Message.Sender.ID)
			if _, ok := runningCaps[messageId]; !ok {
				log.Println("session is not found")
				return
			}
			if c.Sender.ID != runningCaps[messageId].userID {
				b.Respond(c, &tb.CallbackResponse{Text: "Not for you"})
				return
			}
			sz := len(runningCaps[messageId].userInput)
			if sz > 0 {
				runningCaps[messageId].userInput = runningCaps[messageId].userInput[:sz-1]
			}
			b.Respond(c, &tb.CallbackResponse{Text: "Input: " + runningCaps[messageId].userInput})

		})
		var buff bytes.Buffer
		id := cap.GetNewCaptcha(&buff)
		runningCaps[messageId] = &caps{
			captchaId:      id,
			userInput:      "",
			userID:         m.Sender.ID,
			chatID:         chatID,
			loginMessageID: messageIdOrig,
			success:        false,
		}
		img := &tb.Photo{File: tb.FromReader(&buff), Caption: "оставь надежду всяк сюда входящий"}
		msg, _ := b.Reply(m, img, menu)
		mi, _ := msg.MessageSig()
		runningCaps[messageId].captchaMessageID = mi
		go func(c *caps) {
			<-time.After(1 * time.Minute)
			if !c.success {
				log.Println("timeout clean")
				// delete captcha
				b.Delete(&tb.StoredMessage{MessageID: c.captchaMessageID, ChatID: c.chatID})
				// delete join message
				b.Delete(&tb.StoredMessage{MessageID: c.loginMessageID, ChatID: c.chatID})
			}
			delete(runningCaps, c.loginMessageID+strconv.FormatInt(c.chatID, 10))
		}(runningCaps[messageId])
	})

	go b.Start()

	select {
	case <-ctx.Done():
		log.Println("Global shutdown")
	case <-intCh:
		log.Println("Gracefully stopping... (press Ctrl+C again to force)")
	}
	cancel()
	b.Stop()

}
