package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis"
	_ "github.com/lib/pq"
	"html/template"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

const DEBUG = false

type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
}

type DatabaseConfig struct {
	Format string
	Url    string
}

type RedisConfig struct {
	Address  string
	Password string
}

func NewConfigFromEnv() Config {
	config := Config{}

	config.Database.Format = os.Getenv("KSTATS_DATABASE_TYPE")
	config.Database.Url = os.Getenv("KSTATS_DATABASE_URL")

	config.Redis.Address = os.Getenv("KSTATS_REDIS_ADDRESS")
	config.Redis.Password = os.Getenv("KSTATS_REDIS_PASSWORD")

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
	Id                int
	Name              string
	Lines             int
	Words             int
	WordsPerLine      float64
	CharactersPerLine float64
	HourUsage         []float64
	Users             []UserData
	Questions         []FloatEntry
	Exclamations      []FloatEntry
	Caps              []FloatEntry
	EmojiHappy        []FloatEntry
	EmojiSad          []FloatEntry
	LongestLines      []FloatEntry
	ShortestLines     []FloatEntry
	TotalWords        []TotalEntry
	AverageWords      []FloatEntry
	References        []ReferenceData
}

type FloatEntry struct {
	Name  string
	Value float64
}

type IntEntry struct {
	Name  string
	Value int
}

type TotalEntry struct {
	Name     string
	Value    int
	Previous string
}

type UserData struct {
	Name         string
	Lines        int
	Words        int
	WordsPerLine float64
	LastSeen     time.Time
}

type ReferenceData struct {
	Name     string
	LastUsed string
	Count    string
}

func handleError(err error) {
	if DEBUG {
		panic(err)
	} else {
		println(err.Error())
	}
}

func main() {
	config := NewConfigFromEnv()

	var redisClient *redis.Client
	redisClient = redis.NewClient(&redis.Options{
		Addr:     config.Redis.Address,
		Password: config.Redis.Password,
	})

	db, err := sql.Open(config.Database.Format, config.Database.Url)
	if err != nil {
		panic(err)
	}

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, channel := path.Split(r.URL.Path)
		if strings.HasPrefix(channel, "#") {
			var channelData ChannelData
			data, err := redisClient.Get(channel).Bytes()
			if err == nil {
				err = json.Unmarshal(data, &channelData)
			}

			if err != nil {
				channelData, err = buildChannelData(db, channel)
				if err != nil {
					handleError(err)
					return
				}
				data, err = json.Marshal(channelData)
				err = redisClient.Set(channel, data, time.Minute*5).Err()
				if err != nil {
					handleError(err)
					return
				}
			}

			err = formatTemplate(w, "statistics", channelData)
			if err != nil {
				handleError(err)
				return
			}
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

func buildChannelData(db *sql.DB, channel string) (channelData ChannelData, err error) {
	err = db.QueryRow("SELECT id, channel FROM channels WHERE channel ILIKE $1", channel).Scan(&channelData.Id, &channelData.Name)
	if err != nil {
		return
	}

	err = db.QueryRow("SELECT COUNT(*), SUM(words), AVG(words), AVG(characters) FROM messages WHERE channel = $1", channelData.Id).Scan(&channelData.Lines, &channelData.Words, &channelData.WordsPerLine, &channelData.CharactersPerLine)
	if err != nil {
		return
	}

	channelData.HourUsage, err = retrieveHourUsage(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.Users, err = retrieveUsers(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.Questions, err = retrievePercentageStats(db, channelData.Id, "question")
	if err != nil {
		return
	}

	channelData.Exclamations, err = retrievePercentageStats(db, channelData.Id, "exclamation")
	if err != nil {
		return
	}

	channelData.Caps, err = retrievePercentageStats(db, channelData.Id, "caps")
	if err != nil {
		return
	}

	channelData.EmojiHappy, err = retrievePercentageStats(db, channelData.Id, "emoji_happy")
	if err != nil {
		return
	}

	channelData.EmojiSad, err = retrievePercentageStats(db, channelData.Id, "emoji_sad")
	if err != nil {
		return
	}

	channelData.LongestLines, err = retrieveLongestLines(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.ShortestLines, err = retrieveShortestLines(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.TotalWords, err = retrieveTotalWords(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.References, err = retrieveReferences(db, channelData.Id)
	if err != nil {
		return
	}

	channelData.AverageWords, err = retrieveAverageWords(db, channelData.Id)
	if err != nil {
		return
	}

	return
}

func retrievePercentageStats(db *sql.DB, channel int, stats string) ([]FloatEntry, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t."+stats+" FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, round((count(nullif(messages."+stats+", false)) * 100) :: numeric / count(*)) as "+stats+" FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash DESC) t LEFT JOIN users ON t.hash = users.hash WHERE t."+stats+" > 0 ORDER BY "+stats+" LIMIT $2;", channel, 2)
	if err != nil {
		return nil, err
	}
	var data []FloatEntry
	for result.Next() {
		var info FloatEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}

func retrieveHourUsage(db *sql.DB, channel int) ([]float64, error) {
	result, err := db.Query("SELECT coalesce(count, 0) AS count FROM generate_series(0, 23, 1) AS series LEFT OUTER JOIN (SELECT EXTRACT(HOUR FROM time) as hour, count(*) as count FROM messages WHERE channel = $1 GROUP BY hour) results ON (series = results.hour)", channel)
	if err != nil {
		return nil, err
	}
	var data []int
	max := 0
	for result.Next() {
		var info int
		err := result.Scan(&info)
		if err != nil {
			panic(err)
		}
		if info > max {
			max = info
		}
		data = append(data, info)
	}
	var normalizedResult []float64
	for _, element := range data {
		normalizedResult = append(normalizedResult, float64(element)/float64(max)*100.0)
	}
	return normalizedResult, nil
}

func retrieveLongestLines(db *sql.DB, channel int) ([]FloatEntry, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.average FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, avg(messages.characters) as average FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash) t LEFT JOIN users ON t.hash = users.hash ORDER BY average DESC LIMIT $2;", channel, 2)
	if err != nil {
		return nil, err
	}
	var data []FloatEntry
	for result.Next() {
		var info FloatEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}

func retrieveShortestLines(db *sql.DB, channel int) ([]FloatEntry, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.average FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, avg(messages.characters) as average FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash) t LEFT JOIN users ON t.hash = users.hash ORDER BY average ASC LIMIT $2;", channel, 2)
	if err != nil {
		return nil, err
	}
	var data []FloatEntry
	for result.Next() {
		var info FloatEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}

func retrieveTotalWords(db *sql.DB, channel int) ([]TotalEntry, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.words FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, SUM(messages.words) as words FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash) t LEFT JOIN users ON t.hash = users.hash ORDER BY words DESC LIMIT $2", channel, 2)
	if err != nil {
		return nil, err
	}
	var data []TotalEntry
	previous := ""
	for result.Next() {
		var info TotalEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		info.Previous = previous
		previous = info.Name
		data = append(data, info)
	}
	return data, nil
}

func retrieveAverageWords(db *sql.DB, channel int) ([]FloatEntry, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.words FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, AVG(messages.words) as words FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash) t LEFT JOIN users ON t.hash = users.hash ORDER BY words DESC LIMIT $2", channel, 2)
	if err != nil {
		return nil, err
	}
	var data []FloatEntry
	for result.Next() {
		var info FloatEntry
		err := result.Scan(&info.Name, &info.Value)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}

func retrieveUsers(db *sql.DB, channel int) ([]UserData, error) {
	result, err := db.Query("SELECT coalesce(users.nick, '[Unknown]'), t.lines, t.words, t.wordsPerLine, t.lastSeen FROM (SELECT coalesce(groups.\"group\", messages.sender) AS hash, COUNT(*) as lines, SUM(messages.words) as words, AVG(messages.words) as wordsPerLine, MAX(messages.time) AS lastSeen FROM messages LEFT JOIN groups ON messages.sender = groups.nick AND groups.channel = $1 WHERE messages.channel = $1 GROUP BY hash) t LEFT JOIN users ON t.hash = users.hash ORDER BY lines DESC LIMIT $2", channel, 20)
	if err != nil {
		return nil, err
	}
	var data []UserData
	for result.Next() {
		var info UserData
		err := result.Scan(&info.Name, &info.Lines, &info.Words, &info.WordsPerLine, &info.LastSeen)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}

func retrieveReferences(db *sql.DB, channel int) ([]ReferenceData, error) {
	result, err := db.Query("SELECT coalesce(u1.nick, '[Unknown]') as target,  t.count,  coalesce(u2.nick, '[Unknown]') AS lastUsed FROM (SELECT coalesce(g1.\"group\", t.target)            AS target,  t.count,  coalesce(g2.\"group\", \"references\".source) AS lastUsed FROM (SELECT \"references\".target AS target,  COUNT(*)            as count,  MAX(id)             AS lastUsed FROM \"references\" WHERE \"references\".channel = $1 GROUP BY target) t JOIN \"references\" ON t.lastUsed = \"references\".id LEFT JOIN groups g1 ON \"references\".source = g1.nick AND g1.channel = $1 LEFT JOIN groups g2 ON t.target = g2.nick AND g2.channel = $1) t LEFT JOIN users u1 ON t.target = u1.hash LEFT JOIN users u2 ON t.lastUsed = u2.hash ORDER BY count DESC LIMIT $2", channel, 5)
	if err != nil {
		return nil, err
	}
	var data []ReferenceData
	for result.Next() {
		var info ReferenceData
		err := result.Scan(&info.Name, &info.Count, &info.LastUsed)
		if err != nil {
			panic(err)
		}
		data = append(data, info)
	}
	return data, nil
}
