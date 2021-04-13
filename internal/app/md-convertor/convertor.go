package md_convertor

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
	"github.com/pkg/errors"
	md "github.com/JohannesKaufmann/html-to-markdown"
)

type PageMeta struct {
	Title       string
	Length      int
	SiteName    string
	Image       string
}

func MdFromUrl(rawUrl string) (string, PageMeta, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", PageMeta{}, errors.Wrap(err, "MdFromUrl: invalid url")
	}

	article, err := readability.FromURL(rawUrl, time.Minute)
	if err != nil {
		return "", PageMeta{}, errors.Wrap(err, "MdFromUrl: can't read article")
	}

	if u.Host == "github.com" {
		markdown, err := extractReadme(u)
		if err == nil && markdown != "" {
			return markdown, PageMeta{
				Title:    article.Title,
				Length:   article.Length,
				SiteName: article.SiteName,
				Image:    article.Image,
			}, nil
		}
	}

	converter := md.NewConverter(u.Host, true, nil)
	markdown, err := converter.ConvertString(article.Content)
	if err != nil {
		return "", PageMeta{}, errors.Wrap(err, "MdFromUrl: can't convert article to md")
	}

	return markdown, PageMeta{
		Title:    article.Title,
		Length:   article.Length,
		SiteName: article.SiteName,
		Image:    article.Image,
	}, nil
}

func extractReadme(u *url.URL) (string, error) {
	pathParts := strings.Split(u.Path, "/")
	if len(pathParts) > 2 {
		repoRootPath := fmt.Sprintf("%s/%s", pathParts[1], pathParts[2])
		for _, branch := range []string{"master", "main", "develop", "development", "dev"} {
			readmeURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/README.md", repoRootPath, branch)
			markdownResp, err := http.Get(readmeURL)
			if err == nil && markdownResp.StatusCode == 200 {
				bodyBytes, err := ioutil.ReadAll(markdownResp.Body)
				if err != nil {
					return "", errors.Wrapf(err, "extractReadme: can't read response body from %s", readmeURL)
				}
				return string(bodyBytes), nil
			}
		}
	}
	return "", nil
}