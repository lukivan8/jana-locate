package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/lukivan8/jana-address/pkg/api"
	"github.com/lukivan8/jana-address/pkg/client"
)

func main() {
	url := os.Getenv("JANA_API_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	out, err := client.New(url).Normalize(context.Background(), api.NormalizeRequest{Records: []api.InputRecord{{Id: "delivery-1", RawAddress: "Туран 55/6 кв 99"}}})
	if err != nil {
		log.Fatal(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(out); err != nil {
		log.Fatal(err)
	}
}
