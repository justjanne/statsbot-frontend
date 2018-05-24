package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"html/template"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
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
	pageTemplate, err := template.ParseFiles(fmt.Sprintf("templates/%s.html", templateName))
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
	Users           []UserData
	Questions       []PercentageEntry
	Exclamations    []PercentageEntry
	Caps            []PercentageEntry
	EmojiHappy      []PercentageEntry
	EmojiSad        []PercentageEntry
	LongestLines    []TotalEntry
	ShortestLines   []TotalEntry
	Total           []TotalEntry
	Average         TotalEntry
	ChannelAverage  TotalEntry
}

type PercentageEntry struct {
	Name  string
	Value float64
}

type TotalEntry struct {
	Name  string
	Value int
}

type UserData struct {
	Name     string
	Total    int
	Words    int
	LastSeen time.Time
}

func main() {
	config := NewConfigFromEnv()

	db, err := sql.Open(config.Database.Format, config.Database.Url)
	if err != nil {
		panic(err)
	}

	assets := http.FileServer(http.Dir("assets"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, channel := path.Split(r.URL.Path)
		if strings.HasPrefix(channel, "#") {
			channelData := ChannelData{}
			err = db.QueryRow("SELECT id, channel FROM channels WHERE channel ILIKE $1", channel).Scan(&channelData.Id, &channelData.Name)
			if err != nil {
				println(err.Error())
				return
			}
			err = db.QueryRow("SELECT SUM(characters), SUM(words) FROM messages WHERE channel = $1", channelData.Id).Scan(&channelData.TotalCharacters, &channelData.TotalWords)
			if err != nil {
				println(err.Error())
				return
			}
			result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.characters, t.words, t.lastSeen FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, SUM(messages.characters) as characters, SUM(messages.words) as words, MAX(messages.time) AS lastSeen FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = 1 WHERE messages.channel = 1 GROUP BY hash ORDER BY characters DESC) t LEFT JOIN users ON t.hash = users.hash LIMIT 20")
			if err != nil {
				println(err.Error())
				return
			}
			for result.Next() {
				var info UserData
				err := result.Scan(&info.Name, &info.Total, &info.Words, &info.LastSeen)
				if err != nil {
					panic(err)
				}
				channelData.Users = append(channelData.Users, info)
			}

			channelData.Questions, err = retrievePercentageStats(db, "question")
			if err != nil {
				println(err.Error())
				return
			}

			channelData.Exclamations, err = retrievePercentageStats(db, "exclamation")
			if err != nil {
				println(err.Error())
				return
			}

			channelData.Caps, err = retrievePercentageStats(db, "caps")
			if err != nil {
				println(err.Error())
				return
			}

			err = formatTemplate(w, "statistics", channelData)
			if err != nil {
				println(err.Error())
				return
			}
		} else {
			w.Header().Set("Vary", "Accept-Encoding")
			w.Header().Set("Cache-Control", "public, max-age=31536000")
			assets.ServeHTTP(w, r)
		}
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}

func retrievePercentageStats(db *sql.DB, stats string) ([]PercentageEntry, error) {
	var data []PercentageEntry
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t." + stats + " FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, round((count(nullif(messages." + stats + ", false)) * 100) :: numeric / count(*)) as " + stats + " FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = 1 WHERE messages.channel = 1 GROUP BY hash ORDER BY " + stats + " DESC) t LEFT JOIN users ON t.hash = users.hash LIMIT 2;")
	if err != nil {
		return nil, err
	}
	for result.Next() {
		var info PercentageEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}
