# :link: RSSHub Lite

Crawl web pages to RSS feeds.

## Guide

### Docker compose

```yaml
version: '3.4'
services:
  miniflux:
    image: ghcr.io/naiba/rsshub-lite:latest
    ports:
      - "3000:3000"
    volumes:
        - ./data:/rsshub-lite/data
```

### Config file

```shell
root@localhost:/root/rsshub-lite$ tree
.
├── data
│   └── config.json
└── docker-compose.yml
```

config.json

```json
{
    "sources": [
        {
            "name": "登链社区最新文章",
            "slug": "learnblockchain-newest",
            "url": "https://learnblockchain.cn/categories/all/newest/",
            "item_selector": "div.stream-list.blog-stream>section.stream-list-item",
            "title_selector": "div.summary>h2.title",
            "description_selector": "div.summary>div.excerpt>p",
            "link_selector": "div.summary>h2.title>a",
            "next_page_match": "class=\"page-link\" href=\"(.*)\" rel=\"next\"",
            "delay": 3600
        }
    ],
    "username": "naiba", # Enable HTTP Basic Auth if not empty
    "password": "abian",
    "max_items": 25
}
```
