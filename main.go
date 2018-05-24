package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"html/template"
	"net/http"
	"os"
	"path"
)

type Config struct {
	Database DatabaseConfig
}

type DatabaseConfig struct {
	Format string
	Url    string
}

func NewConfigFromEnv() Config {
	config := Config{}

	config.Database.Format = os.Getenv("KSTATS_DATABASE_TYPE")
	config.Database.Url = os.Getenv("KSTATS_DATABASE_URL")

	return config
}

func formatTemplate(w http.ResponseWriter, templateName string, data interface{}) error {
	pageTemplate, err := template.ParseFiles(fmt.Sprintf("templates/%s", templateName))
	if err != nil {
		return err
	}

	err = pageTemplate.Execute(w, data)
	if err != nil {
		return err
	}

	return nil
}

type ChannelData struct {
	Id              int
	Name            string
	TotalWords      int
	TotalCharacters int
}

func main() {
	config := NewConfigFromEnv()

	db, err := sql.Open(config.Database.Format, config.Database.Url)
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, channel := path.Split(r.URL.Path)
		channelData := ChannelData{}
		db.QueryRow("SELECT id, channel FROM channels WHERE channel ILIKE $1", channel).Scan(&channelData.Id, &channelData.Name)
		db.QueryRow("SELECT SUM(characters), SUM(words) FROM messages WHERE channel = $1").Scan(&channelData.TotalCharacters, &channelData.TotalWords)
		formatTemplate(w, "statistics", channelData)
	})
}
