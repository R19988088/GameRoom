package main

import (
	"log"

	"github.com/R19988088/GameRoom/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
