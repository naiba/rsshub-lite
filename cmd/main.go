package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	cbp "github.com/DaRealFreak/cloudflare-bp-go"
	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
	"github.com/gorilla/feeds"
	"github.com/robfig/cron/v3"
	"github.com/samber/mo"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"

	"github.com/naiba/rsshub-lite/internal/model"
)

var bypassCloudFlareHttpClient = &http.Client{
	Transport: cbp.AddCloudFlareByPass(http.DefaultTransport),
}
var feedList map[string]*feeds.Feed
var feedListLock sync.RWMutex
var config model.Config
var jobManager *cron.Cron
var feedLimiter singleflight.Group

func main() {
	configRes := getConfig()
	if configRes.IsError() {
		panic(configRes.Error())
	}
	config = configRes.MustGet()
	feedList = loadCache()

	if err := initJobs(); err != nil {
		panic(err)
	}

	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{
		Views: engine,
	})
	app.Use(recover.New())

	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index", fiber.Map{
			"Sources": config.Sources,
		})
	})

	feed := app.Group("/feed")
	if len(config.Username) != 0 {
		feed.Use(basicauth.New(basicauth.Config{
			Users: map[string]string{
				config.Username: config.Password,
			},
		}))
	}
	feed.Get("/:slug", func(c *fiber.Ctx) error {
		slug := c.Params("slug")

		content, err, _ := feedLimiter.Do(slug, func() (any, error) {
			var content string
			var err error
			feedListLock.RLock()
			if feed, has := feedList[slug]; has {
				content, err = feed.ToRss()
				if err != nil {
					feedListLock.RUnlock()
					return content, err
				}
			}
			feedListLock.RUnlock()
			return content, err
		})

		if err != nil {
			return err
		}

		c.Response().Header.Add("Content-Type", "application/xml")
		_, err = c.WriteString(content.(string))
		return err
	})

	app.Listen(":3000")
}

func getConfig() mo.Result[model.Config] {
	content, err := os.ReadFile("data/config.json")
	if err != nil {
		return mo.Err[model.Config](err)
	}
	var config model.Config
	if err := json.Unmarshal(content, &config); err != nil {
		return mo.Err[model.Config](err)
	}
	return mo.Ok(config)
}

func loadCache() map[string]*feeds.Feed {
	cacheContent, err := os.ReadFile("data/cache.json")
	if err != nil {
		return make(map[string]*feeds.Feed)
	}

	var list map[string]*feeds.Feed
	if err := json.Unmarshal(cacheContent, &list); err != nil {
		return make(map[string]*feeds.Feed)
	}

	return list
}

func initJobs() error {
	jobManager = cron.New()
	for i := 0; i < len(config.Sources); i++ {
		source := config.Sources[i]
		if _, has := feedList[source.Slug]; !has {
			feedList[source.Slug] = &feeds.Feed{
				Title: source.Name,
				Link:  &feeds.Link{Href: source.Url},
			}
		}

		fn := func() {
			if err := refreshFeed(&source); err != nil {
				log.Printf("[error] refresh feed %s failed: %s\n", source.Name, err)
			}
		}

		go fn()
		_, err := jobManager.AddFunc(fmt.Sprintf("@every %ds", source.Delay), fn)

		if err != nil {
			return err
		}
	}
	jobManager.Start()
	return nil
}

func refreshFeed(source *model.Source) error {
	var items []*feeds.Item
	var err error
	if items, err = matchFeedItems(source.Url, source, make([]*feeds.Item, 0)); err != nil {
		return err
	}

	if len(items) == 0 {
		return fmt.Errorf("[error] %s no items found", source.Name)
	}

	feedListLock.Lock()
	defer feedListLock.Unlock()
	feedList[source.Slug].Items = items

	cacheContent, err := json.Marshal(feedList)
	if err != nil {
		return err
	}
	if err := os.WriteFile("data/cache.json", cacheContent, 0644); err != nil {
		return err
	}

	return nil
}

func matchFeedItems(targetUrl string, source *model.Source, items []*feeds.Item) ([]*feeds.Item, error) {
	resp, err := bypassCloudFlareHttpClient.Get(targetUrl)
	if err != nil {
		return items, err
	}
	if resp.StatusCode != 200 {
		return items, fmt.Errorf("[error] %s -> %s status code: %d %s", source.Name, targetUrl, resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()

	switch strings.Split(resp.Header.Get("Content-Type"), ";")[0] {
	case "application/json":
		return procJsonResponse(targetUrl, resp, source, items)
	case "text/html":
		return procHtmlResponse(targetUrl, resp, source, items)
	}

	return items, fmt.Errorf("[error] %s -> %s unsupported content type: %s", source.Name, targetUrl, resp.Header.Get("Content-Type"))
}

func procJsonResponse(targetUrl string, resp *http.Response, source *model.Source, items []*feeds.Item) ([]*feeds.Item, error) {
	contentBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return items, err
	}
	content := string(contentBytes)
	gjson.Get(content, source.ItemSelector).ForEach(func(key, value gjson.Result) bool {
		var item feeds.Item
		if len(source.TitleSelector) > 0 {
			item.Title = value.Get(source.TitleSelector).String()
		}
		if len(source.LinkSelector) > 0 {
			item.Link = &feeds.Link{Href: value.Get(source.LinkSelector).String()}
		}
		if len(source.DescriptionSelector) > 0 {
			item.Description = value.Get(source.DescriptionSelector).String()
		}
		if len(item.Title) > 0 && len(item.Link.Href) > 0 {
			items = append(items, &item)
		}
		return len(items) < config.MaxItems
	})

	if len(items) < config.MaxItems && len(source.NextPageMatch) > 0 {
		res := gjson.Get(content, source.NextPageMatch)
		if res.Exists() {
			return matchFeedItems(mergeUrl(targetUrl, res.String()), source, items)
		}
	}

	return items, nil
}

func procHtmlResponse(targetUrl string, resp *http.Response, source *model.Source, items []*feeds.Item) ([]*feeds.Item, error) {
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return items, err
	}

	doc.Find(source.ItemSelector).EachWithBreak(func(i int, s *goquery.Selection) bool {
		var item feeds.Item
		if len(source.TitleSelector) > 0 {
			titleSel := s.Find(source.TitleSelector).First()
			item.Title = titleSel.Text()
		}
		if len(source.LinkSelector) > 0 {
			linkSel := s.Find(source.LinkSelector).First()
			item.Link = &feeds.Link{Href: linkSel.AttrOr("href", "")}
		}
		if len(source.DescriptionSelector) > 0 {
			descSel := s.Find(source.DescriptionSelector).First()
			item.Description = descSel.Text()
		}
		if len(item.Title) > 0 && len(item.Link.Href) > 0 {
			items = append(items, &item)
		}
		return len(items) < config.MaxItems
	})

	if len(items) < config.MaxItems && len(source.NextPageMatch) > 0 {
		body, err := doc.Html()
		if err != nil {
			return items, err
		}
		matcher := regexp.MustCompile(source.NextPageMatch)
		matches := matcher.FindStringSubmatch(body)
		if len(matches) > 1 {
			nextPageUrl := matches[1]
			return matchFeedItems(mergeUrl(targetUrl, nextPageUrl), source, items)
		}
	}

	return items, err
}

func mergeUrl(base, target string) string {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	if strings.HasPrefix(target, "//") {
		u, err := url.Parse(base)
		if err != nil {
			return target
		}
		return u.Scheme + ":" + target
	}
	if strings.HasPrefix(target, "/") {
		u, err := url.Parse(base)
		if err != nil {
			return target
		}
		return u.Scheme + "://" + u.Host + target
	}
	if strings.HasSuffix(base, "/") {
		return base + target
	}
	return base + "/" + target
}
