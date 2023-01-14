# :link: RSSHub Lite

Crawl web pages to RSS feeds.

## Guide

### Layout

```shell
root@localhost:/root/rsshub-lite$ tree
.
├── data
│   └── config.json
└── docker-compose.yml
```

### docker-compose.yml

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

### config.json

Checkout `data/config.json.example`

```json
{
    "sources": [
        {
            "name": "登链社区最新文章",
            /* ... */
        }
    ],
    "username": "naiba", /* Enable HTTP-Basic-Auth for /feed/* if not empty */
    "password": "abian",
    "max_items": 25
}
```
