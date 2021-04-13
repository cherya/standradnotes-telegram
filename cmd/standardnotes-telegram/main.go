package main

import (
	"fmt"
	"github.com/cherya/standardnotes-telegram/internal/app/bot"
	md_convertor "github.com/cherya/standardnotes-telegram/internal/app/md-convertor"
	"github.com/cherya/standardnotes-telegram/internal/pkg/sn"
	"github.com/pkg/errors"
	"log"
	"strconv"
	"strings"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
	"mvdan.cc/xurls/v2"
)

func main() {
	envs, err := godotenv.Read(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	endpoint := envs["STANDARDNOTES_ENDPOINT"]
	email := envs["STANDARDNOTES_EMAIL"]
	password := envs["STANDARDNOTES_PASSWORD"]
	botToken := envs["TELEGRAM_BOT_TOKEN"]
	ownerID, err := strconv.Atoi(envs["TELEGRAM_OWNER_ID"])
	if err != nil {
		log.Println("Owner id is not set, bot will process messages from anyone")
	}

	snn, err := sn.New(endpoint)
	if err != nil {
		log.Fatal(err)
	}

	a := app{
		sn:       snn,
		email:    email,
		password: password,
	}

	b, err := bot.New(botToken, []int{ownerID}, a.handleBotMessage)
	if err != nil {
		log.Fatal(err)
	}

	err = b.Start()
	if err != nil {
		log.Fatal(err)
	}
}

type app struct {
	sn       *sn.StandardNotes
	email    string
	password string
}

func (a *app) handleBotMessage(m *tgbotapi.Message) (string, error) {
	err := a.sn.Login(a.email, a.password)
	if err != nil {
		return "", err
	}
	err = a.sn.Sync()
	if err != nil {
		return "", err
	}

	messageText := getMessageText(m)
	messageLinks := getMessageLinks(m)
	messageTitle := getMessageTitle(m)
	messageTags := getMessageTags(m)

	if len(messageLinks) > 0 {
		messageTags = append(messageTags, "links")
	}
	if len(messageLinks) == 1 {
		var meta md_convertor.PageMeta
		messageText, meta, err = md_convertor.MdFromUrl(messageLinks[0])
		if err != nil {
			return "", errors.Wrap(err, "handleBotMessage: error getting url content")
		}
		messageTitle = meta.Title
		messageText = fmt.Sprintf("Original: %s  \n  \n%s", messageLinks[0], messageText)
	}

	_, err = a.sn.AddNote(messageTitle, messageText, messageTags)
	if err != nil {
		return "", err
	}

	a.sn.Logout()

	tgTags := strings.Builder{}
	for _, t := range messageTags {
		tgTags.WriteString(fmt.Sprintf("#%s ", t))
	}

	return fmt.Sprintf("Note *«%s»* created \nTags: %s", messageTitle, tgTags.String()), nil
}

func getMessageText(msg *tgbotapi.Message) string {
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	return text
}

func getMessageTitle(msg *tgbotapi.Message) string {
	title := ""
	if msg.ForwardFromChat != nil {
		title = fmt.Sprintf("%s #%d", msg.ForwardFromChat.Title, msg.ForwardFromMessageID)
	}
	return title
}

func getMessageTags(msg *tgbotapi.Message) []string {
	tags := []string{"telegram", "inbox"}
	msgText := getMessageText(msg)
	if msg.Entities != nil {
		for _, e := range *msg.Entities {
			if e.Type == "hashtag" {
				utfEncodedString := utf16.Encode([]rune(msgText))
				runeString := utf16.Decode(utfEncodedString[e.Offset+1 : e.Offset+e.Length])
				hashtag := string(runeString)
				tags = append(tags, "telegram."+hashtag)
			}
		}
	}

	return tags
}

func getMessageLinks(msg *tgbotapi.Message) []string {
	links := make([]string, 0)
	msgText := getMessageText(msg)
	if msg.Entities != nil {
		for _, e := range *msg.Entities {
			if e.URL != "" {
				links = append(links, e.URL)
			}
		}
	}

	rxRelaxed := xurls.Relaxed()
	found := rxRelaxed.FindAllString(msgText, -1)
	if len(found) > 0 {
		links = append(links, found...)
	}

	return links
}
