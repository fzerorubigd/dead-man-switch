package main

import (
	"encoding/xml"
	"flag"
	"log"
	"time"

	"github.com/fzerorubigd/dead-man-switch/internal/cli"
	"github.com/fzerorubigd/dead-man-switch/internal/fetch"
)

type Rss struct {
	XMLName  xml.Name `xml:"rss"`
	Text     string   `xml:",chardata"`
	Version  string   `xml:"version,attr"`
	Webfeeds string   `xml:"webfeeds,attr"`
	Media    string   `xml:"media,attr"`
	Channel  struct {
		Text        string `xml:",chardata"`
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Link        string `xml:"link"`
		Image       struct {
			Text  string `xml:",chardata"`
			URL   string `xml:"url"`
			Title string `xml:"title"`
			Link  string `xml:"link"`
		} `xml:"image"`
		LastBuildDate string `xml:"lastBuildDate"`
		Icon          string `xml:"icon"`
		Generator     string `xml:"generator"`
		Item          []struct {
			Text string `xml:",chardata"`
			Guid struct {
				Text        string `xml:",chardata"`
				IsPermaLink string `xml:"isPermaLink,attr"`
			} `xml:"guid"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			Content     struct {
				Text     string `xml:",chardata"`
				URL      string `xml:"url,attr"`
				Type     string `xml:"type,attr"`
				FileSize string `xml:"fileSize,attr"`
				Medium   string `xml:"medium,attr"`
				Rating   struct {
					Text   string `xml:",chardata"`
					Scheme string `xml:"scheme,attr"`
				} `xml:"rating"`
				Description struct {
					Text string `xml:",chardata"`
					Type string `xml:"type,attr"`
				} `xml:"description"`
			} `xml:"content"`
			Category string `xml:"category"`
		} `xml:"item"`
	} `xml:"channel"`
}

func main() {
	rss := flag.String("rss", "https://rubi.gd/@fzero.rss", "The profile rss")
	idl := flag.Duration("idle", time.Hour*24*30, "allowed idle time")
	flag.Parse()

	ctx, cnl := cli.Context()
	defer cnl()

	var parsed Rss
	err := fetch.XML(ctx, *rss, &parsed)
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range parsed.Channel.Item {
		ts, err := time.Parse(time.RFC1123Z, item.PubDate)
		if err != nil {
			continue
		}

		if time.Since(ts) < *idl {
			log.Printf("You were alive at %s", ts.Format(time.RFC3339))
			return
		}
	}

	log.Fatal("Are you dead?")
}
