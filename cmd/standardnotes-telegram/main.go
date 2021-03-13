package main

import (
	"fmt"
	"github.com/cherya/standardnotes-telegram/internal/pkg/sn"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf16"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
	"golang.org/x/net/html"
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

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 20
	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}
		if ownerID != 0 && update.Message.From.ID != ownerID {
			continue
		}

		log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

		err = snn.Login(email, password)
		if err != nil {
			log.Println(err)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			bot.Send(msg)
		}
		err = snn.Sync()
		if err != nil {
			log.Println(err)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			bot.Send(msg)
		}

		messageText := getMessageText(update.Message)
		messageLinks := getMessageLinks(update.Message)
		messageTitle := getMessageTitle(update.Message, messageLinks)
		messageTags := getMessageTags(update.Message, messageLinks)

		_, err = snn.AddNote(messageTitle, messageText, messageTags)
		if err != nil {
			log.Println(err)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			bot.Send(msg)
		}

		tgTags := strings.Builder{}
		for _, t  := range messageTags {
			tgTags.WriteString(fmt.Sprintf("#%s ", t))
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Note «<b>%s</b>» created \nTags: %s", messageTitle, tgTags.String()))
		msg.ParseMode = tgbotapi.ModeHTML
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}
	}
}

func getMessageText(msg *tgbotapi.Message) string {
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	return text
}

func getMessageTitle(msg *tgbotapi.Message, urls []string) string {
	title := ""
	if msg.ForwardFromChat != nil {
		title = msg.ForwardFromChat.Title
	}
	if len(urls) > 0 {
		resp, err := http.Get(urls[0])
		if err != nil {
			log.Println(err)
			return title
		}
		meta := extractHTMLMeta(resp.Body)
		if meta.Title != "" {
			return meta.Title
		}
	}
	return title
}

func getMessageTags(msg *tgbotapi.Message, urls []string) []string {
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

	if len(urls) > 0 {
		tags = append(tags, "links")
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

type HTMLMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"site_name"`
}

func extractHTMLMeta(resp io.Reader) *HTMLMeta {
	z := html.NewTokenizer(resp)

	titleFound := false

	hm := new(HTMLMeta)

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return hm
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			if t.Data == `body` {
				return hm
			}
			if t.Data == "title" {
				titleFound = true
			}
			if t.Data == "meta" {
				desc, ok := extractMetaProperty(t, "description")
				if ok {
					hm.Description = desc
				}

				ogTitle, ok := extractMetaProperty(t, "og:title")
				if ok {
					hm.Title = ogTitle
				}

				ogDesc, ok := extractMetaProperty(t, "og:description")
				if ok {
					hm.Description = ogDesc
				}

				ogImage, ok := extractMetaProperty(t, "og:image")
				if ok {
					hm.Image = ogImage
				}

				ogSiteName, ok := extractMetaProperty(t, "og:site_name")
				if ok {
					hm.SiteName = ogSiteName
				}
			}
		case html.TextToken:
			if titleFound {
				t := z.Token()
				hm.Title = t.Data
				titleFound = false
			}
		}
	}
	return hm
}

func extractMetaProperty(t html.Token, prop string) (content string, ok bool) {
	for _, attr := range t.Attr {
		if attr.Key == "property" && attr.Val == prop {
			ok = true
		}

		if attr.Key == "content" {
			content = attr.Val
		}
	}

	return
}