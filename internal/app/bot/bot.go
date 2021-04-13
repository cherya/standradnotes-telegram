package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"log"
)

type MessageHandlerFunc func(message *tgbotapi.Message) (string, error)

type SNBot struct {
	token          string
	owners         []int
	api            *tgbotapi.BotAPI
	messageHandler MessageHandlerFunc
}

func New(token string, owners []int, f MessageHandlerFunc) (*SNBot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, errors.Wrap(err, "New: can't create bot api")
	}
	return &SNBot{
		token:          token,
		api:            api,
		owners:         owners,
		messageHandler: f,
	}, nil
}

func (b *SNBot) IsOwner(id int) bool {
	for _, o := range b.owners {
		if o == id {
			return true
		}
	}
	return false
}

func (b *SNBot) Start() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 20
	updates, err := b.api.GetUpdatesChan(u)
	if err != nil {
		return errors.Wrap(err, "Start: error getting update channel")
	}

	for update := range updates {
		if update.Message == nil { // ignore any non-Message Updates
			continue
		}
		if !b.IsOwner(update.Message.From.ID) {
			continue
		}

		log.Printf("[%s:%d] %s", update.Message.From.UserName, update.Message.From.ID, update.Message.Text)

		respText, err := b.messageHandler(update.Message)
		if err != nil {
			log.Println(err)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			_, err = b.api.Send(msg)
			if err != nil {
				log.Println(err)
			}
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, respText)
		msg.ParseMode = tgbotapi.ModeMarkdown
		_, err = b.api.Send(msg)
		if err != nil {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, err.Error())
			_, err = b.api.Send(msg)
			if err != nil {
				log.Println(err)
			}
		}
	}

	return nil
}
