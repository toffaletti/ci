package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

func handleEvent(event string, body json.RawMessage) {
	log.Printf("event: %v body: %v", event, string(body))
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		event := r.Header.Get("X-Github-Event")
		switch event {
		case "":
			log.Printf("got unknown request")
			r.Write(os.Stderr)
			return
		default:
			raw := json.RawMessage{}
			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("error reading body %v", err)
				return
			}
			err = raw.UnmarshalJSON(data)
			if err != nil {
				panic(err)
			}
			handleEvent(event, raw)
		}
	})
	http.ListenAndServe(":1980", nil)
}
