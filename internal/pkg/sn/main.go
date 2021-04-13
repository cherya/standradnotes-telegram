package sn

import (
	"encoding/json"
	"fmt"
	"github.com/cherya/standardfile/pkg/libsf"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"net/http"
)

type StandardNotes struct {
	client     libsf.Client
	keyChain   libsf.KeyChain
	authParams libsf.Auth
	itemsKeyID string
	tags       map[string]*libsf.Item
	syncToken  string
}

type ref struct {
	UUID        string `json:"uuid"`
	ContentType string `json:"content_type"`
}

func New(endpoint string) (*StandardNotes, error) {
	client, err := libsf.NewClient(http.DefaultClient, libsf.APIVersion20200115, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "New: can't create client")
	}
	sn := &StandardNotes{
		client: client,
		tags:   make(map[string]*libsf.Item),
	}
	return sn, nil
}

func (sn *StandardNotes) Login(email, password string) error {
	var err error
	sn.authParams, err = sn.client.GetAuthParams(email)
	if err != nil {
		return errors.Wrap(err, "sn.Login: can't get auth params")
	}

	if err = sn.authParams.IntegrityCheck(); err != nil {
		return errors.Wrap(err, "sn.Login: integrity check failed")
	}

	sn.keyChain = *sn.authParams.SymmetricKeyPair(password)

	err = sn.client.Login(sn.authParams.Email(), sn.keyChain.Password)
	if err != nil {
		return errors.Wrap(err, "sn.Login: can't login")
	}

	return nil
}

func (sn *StandardNotes) Logout() error {
	return sn.client.Logout()
}

func (sn *StandardNotes) Sync() error {
	items := libsf.NewSyncItems()
	items.ContentType = libsf.ContentTypeItemsKey
	items, err := sn.client.SyncItems(items)
	if err != nil {
		return errors.Wrap(err, "sn.Sync: sync error")
	}

	for _, item := range items.Retrieved {
		if item.ContentType != libsf.ContentTypeItemsKey {
			continue
		}

		// Append `SN|ItemsKey` to the KeyChain.
		err := item.Unseal(&sn.keyChain)
		if err != nil {
			return errors.Wrap(err, "sn.Sync: can't unseal items key")
		}
		sn.itemsKeyID = item.ID
	}

	return nil
}

func (sn *StandardNotes) AddNote(title, text string, tags []string) (string, error) {
	item, err := sn.createNoteItem(title, text)
	if err != nil {
		return "", errors.Wrap(err, "sn.AddNote: can't create item")
	}

	err = sn.syncTags()
	if err != nil {
		return "", errors.Wrap(err, "sn.AddNote: can't sync tags")
	}
	syncItems := libsf.NewSyncItems()
	syncItems.Items = append(syncItems.Items, item)

	for _, t := range tags {
		if tagItem, ok := sn.tags[t]; !ok {
			ti, err := sn.createTagItem(t, item)
			if err != nil {
				return "", errors.Wrap(err, "sn.AddNote: can't create tag item")
			}
			syncItems.Items = append(syncItems.Items, ti)
		} else {
			err = sn.updateTagItem(tagItem, item)
			if err != nil {
				return "", errors.Wrap(err, "sn.AddNote: can't update tag item")
			}
			syncItems.Items = append(syncItems.Items, tagItem)
		}
	}

	_, err = sn.client.SyncItems(syncItems)
	if err != nil {
		return "", errors.Wrap(err, "sn.AddNote: can't sync new note")
	}

	return item.ID, nil
}

func (sn *StandardNotes) newTags(tags []string) []string {
	newTags := make([]string, 0)
	for _, t := range tags {
		if _, ok := sn.tags[t]; !ok {
			newTags = append(newTags, t)
		}
	}
	return newTags
}

func (sn *StandardNotes) createNoteItem(title, text string) (*libsf.Item, error) {
	itemUUID, err := uuid.NewV4()
	if err != nil {
		return nil, errors.Wrap(err, "sn.addNote: can't generate UUID")
	}

	newItem := &libsf.Item{
		ID:          itemUUID.String(),
		ContentType: libsf.ContentTypeNote,
		ItemsKeyID:  sn.itemsKeyID,
		Version:     sn.authParams.Version(),
		AuthParams:  sn.authParams,
		Note: &libsf.Note{
			Title:   title,
			Text:    text,
			AppData: json.RawMessage("{}"),
		},
	}

	newItem.Note.SetUpdatedAtNow()
	err = newItem.Seal(&sn.keyChain)
	if err != nil {
		return nil, errors.Wrap(err, "sn.addNote: can't seal note")
	}

	return newItem, nil
}

func (sn *StandardNotes) syncTags() error {
	items := libsf.NewSyncItems()
	items.ContentType = "Tag"
	items.ComputeIntegrity = true
	items.SyncToken = sn.syncToken

	items, err := sn.client.SyncItems(items)
	if err != nil {
		return errors.Wrap(err, "syncTags: can't sync tags")
	}

	for _, item := range items.Retrieved {
		if item.ContentType != "Tag" {
			continue
		}
		err := item.Unseal(&sn.keyChain)
		if err != nil {
			return errors.Wrap(err, "sn.syncTags: can't unseal tag")
		}
		sn.tags[item.Note.Title] = item
	}

	return nil
}

func (sn *StandardNotes) createTagItem(tag string, itemToAddTag *libsf.Item) (*libsf.Item, error) {
	tagUUID, err := uuid.NewV4()
	if err != nil {
		return nil, errors.Wrap(err, "sn.createTagItem: can't generate UUID")
	}
	newTag := &libsf.Item{
		ID:          tagUUID.String(),
		ContentType: "Tag",
		ItemsKeyID:  sn.itemsKeyID,
		Version:     sn.authParams.Version(),
		AuthParams:  sn.authParams,
		Note: &libsf.Note{
			Title:      tag,
			References: json.RawMessage(fmt.Sprintf("[{\"uuid\":\"%s\",\"content_type\":\"%s\"}]", itemToAddTag.ID, itemToAddTag.ContentType)),
		},
	}
	err = newTag.Seal(&sn.keyChain)
	if err != nil {
		return nil, errors.Wrap(err, "sn.createTagItem: can't seal tag item")
	}
	return newTag, nil
}

func (sn *StandardNotes) updateTagItem(tagItem *libsf.Item, itemToAddTag *libsf.Item) error {
	if tagItem.ContentType != "Tag" {
		return errors.New("sn.updateTagItem: item is not tag")
	}
	refs := make([]ref, 0)
	err := json.Unmarshal(tagItem.Note.References, &refs)
	if err != nil {
		return errors.Wrap(err, "sn.updateTagItem: can't unmarshal references")
	}
	refs = append(refs, ref{
		UUID:        itemToAddTag.ID,
		ContentType: itemToAddTag.ContentType,
	})
	tagItem.Note.References, err = json.Marshal(refs)
	if err != nil {
		return errors.Wrap(err, "sn.updateTagItem: can't marshal references")
	}
	err = tagItem.Seal(&sn.keyChain)
	if err != nil {
		return errors.Wrap(err, "sn.updateTagItem: can't seal tag")
	}
	return nil
}
