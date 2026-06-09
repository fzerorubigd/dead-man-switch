package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/fzerorubigd/gobgg"
	"github.com/fzerorubigd/life-tracker/internal/cli"
)

func main() {
	player := flag.String("player", "fzerorubigd", "the boardgamegeek user name")
	idl := flag.Duration("idle", time.Hour*24*30, "allowed idle time")
	flag.Parse()

	ctx, cnl := cli.Context()
	defer cnl()

	bgg := gobgg.NewBGGClient(gobgg.SetAuthToken(os.Getenv("BGG_TOKEN")))
	plays, err := bgg.Plays(ctx, gobgg.SetUserName(*player))
	if err != nil {
		log.Fatal(err)
	}

	for _, p := range plays.Items {
		if time.Since(p.Date) < *idl {
			log.Printf("You were alive at %s playing %s",
				p.Date.Format(time.RFC3339),
				p.Item.Name)
			return
		}
	}

	log.Fatal("Are you dead?")
}
