package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"

	cbp "github.com/DaRealFreak/cloudflare-bp-go"
	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
	"github.com/gorilla/feeds"
	"github.com/robfig/cron/v3"
	"github.com/samber/mo"
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

func init() {
	configRes := getConfig()
	if configRes.IsError() {
		panic(configRes.Error())
	}
	config = configRes.MustGet()
	feedList = loadCache()
}

func main() {
	if err := initJobs(); err != nil {
		panic(err)
	}

	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{
		Views: engine,
	})
	app.Use(logger.New())
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
				log.Printf("refresh feed %s failed: %s\n", source.Slug, err)
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
		return items, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()

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
		items = append(items, &item)
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
			return matchFeedItems(matches[1], source, items)
		}
	}

	return items, err
}
