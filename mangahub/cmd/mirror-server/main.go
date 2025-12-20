package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	// serves data/mirror.json at GET /titles
	dataPath := "data/mirror.json"

	http.HandleFunc("/titles", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(dataPath)
		if err != nil {
			http.Error(w, "cannot read mirror.json: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// validate JSON so bad file doesn't silently break
		var tmp any
		if err := json.Unmarshal(b, &tmp); err != nil {
			http.Error(w, "mirror.json invalid JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	})

	log.Println("mirror-server listening on http://localhost:9000")
	log.Fatal(http.ListenAndServe(":9000", nil))
}
