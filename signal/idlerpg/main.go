package main

import (
	"encoding/xml"
	"flag"
	"log"

	"github.com/fzerorubigd/dead-man-switch/internal/cli"
	"github.com/fzerorubigd/dead-man-switch/internal/fetch"
)

type Player struct {
	XMLName  xml.Name `xml:"player"`
	Text     string   `xml:",chardata"`
	Username string   `xml:"username"`
	Level    int      `xml:"level"`
	Class    string   `xml:"class"`
	Ttl      string   `xml:"ttl"`
	Userhost string   `xml:"userhost"`
	Online   int      `xml:"online"`
}

func main() {
	player := flag.String("player", "Forud", "the idlerpg player name")
	flag.Parse()

	ctx, cnl := cli.Context()
	defer cnl()

	var parsed Player
	err := fetch.XML(ctx, "https://idlerpg.lolhosting.net/xml.php?player="+*player, &parsed)
	if err != nil {
		log.Fatal(err)
	}

	if parsed.Online > 0 {
		log.Println("You are alive, from the host: ", parsed.Userhost)
		return
	}

	log.Fatal("Are you dead?")
}
