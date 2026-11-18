package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

var userAgent = "AnimeNewsLemporter/0.1.0"

type LemmyData struct {
	Username      string
	Password      string
	Instance      string
	CommunityName string
}

func main() {
	lemmyData := LemmyData{
		Username:      os.Getenv("LEMMY_USERNAME"),
		Password:      os.Getenv("LEMMY_PASSWORD"),
		Instance:      os.Getenv("LEMMY_INSTANCE"),
		CommunityName: os.Getenv("LEMMY_COMMUNITY_NAME"),
	}

	workDir := os.Getenv("WORKING_DIR")

	required := map[string]string{
		"LEMMY_USERNAME":       lemmyData.Username,
		"LEMMY_PASSWORD":       lemmyData.Password,
		"LEMMY_INSTANCE":       lemmyData.Instance,
		"LEMMY_COMMUNITY_NAME": lemmyData.CommunityName,
		"WORKING_DIR":          workDir,
	}

	for name, value := range required {
		if value == "" {
			fmt.Fprintf(os.Stderr, "%s is not set", name)
			os.Exit(1)
		}
	}

	file, err := os.OpenFile(
		filepath.Join(workDir, "aninlem.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file %v", err)
		os.Exit(1)
	}
	defer file.Close()

	var level slog.LevelVar
	level.Set(slog.LevelInfo)

	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{
		Level: &level,
	}))
	slog.SetDefault(logger)

	db := SQLiteDB{}
	if err := db.Init("sqlite", filepath.Join(workDir, "db.sqlite3")); err != nil {
		slog.Error("init database", "error", err)
		file.Close()
		os.Exit(1)
	}
	defer db.DB.Close()

	if err := run(&db, &lemmyData, workDir); err != nil {
		slog.Error("runtime error", "error", err)
		db.DB.Close()
		file.Close()
		os.Exit(1)
	}
}

func run(db *SQLiteDB, lemmyData *LemmyData, workDir string) error {
	blacklist := ReadBlacklist(workDir)

	for _, tag := range []string{"New Anime", "More Info"} {
		if err := FetchNews(db, tag, blacklist); err != nil {
			return err
		}

		if err := db.UpdateTagDate(tag); err != nil {
			return err
		}

		if err := ProcessNews(db, lemmyData); err != nil {
			return err
		}
	}

	return nil
}

func FetchNews(db *SQLiteDB, tag string, blacklist []string) error {
	tagDate, err := db.GetTagDate(tag)
	if err != nil {
		return fmt.Errorf("get tag date for %s: %w", tag, err)
	}

	newsJson, err := DownloadNews(tag, tagDate.Add(-6*time.Hour))
	if err != nil {
		return fmt.Errorf("get news for tag %s: %w", tag, err)
	}

	newsItems, err := ParseTenraiNews(newsJson)
	if err != nil {
		return fmt.Errorf("parse news for tag %s: %w", tag, err)
	}

	for _, newsItem := range newsItems {
		exists, err := db.ExistsNewsEntry(newsItem.MALID)
		if err != nil {
			return fmt.Errorf(
				"check if news entry exists %d: %w",
				newsItem.MALID,
				err,
			)
		}

		if exists {
			continue
		}

		if len(blacklist) > 0 && ContainsAny(newsItem.Title, blacklist) {
			slog.Warn("title matches blacklist entry, skipping", "title", newsItem.Title)
			continue
		}

		if err := db.InsertNewsEntry(newsItem.MALID, newsItem.Title, newsItem.Date); err != nil {
			return fmt.Errorf(
				"insert news item with MAL ID %d: %w",
				newsItem.MALID,
				err,
			)
		}
	}

	return nil
}

func ReadBlacklist(workDir string) []string {
	file, err := os.Open(filepath.Join(workDir, "blacklist.txt"))
	if err != nil {
		slog.Info("no blacklist file found.")
		return []string{}
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Error("read blacklist file", "error", err)
		return []string{}
	}

	return lines
}

func ProcessNews(db *SQLiteDB, lemmyData *LemmyData) error {
	unprocessedNewsEntryIDs, err := db.GetUnprocessedNewsEntryIDs()
	if err != nil {
		return fmt.Errorf(
			"get unprocessed news entry IDs: %w",
			err,
		)
	}

	if len(unprocessedNewsEntryIDs) == 0 {
		return nil
	}

	gl := GoLemmy{}
	if err := gl.Login(lemmyData.Instance, lemmyData.Username, lemmyData.Password); err != nil {
		return fmt.Errorf("lemmy login %w", err)
	}

	for _, newsID := range unprocessedNewsEntryIDs {
		time.Sleep(time.Second)
		newsDetailsJson, err := DownloadNewsDetails(newsID)
		if err != nil {
			slog.Error("download details", "newsID", newsID, "error", err)
			continue
		}

		newsContent, err := ParseTenraiNewsDetails(newsDetailsJson)
		if err != nil {
			slog.Error("parse details", "newsID", newsID, "error", err)
			continue
		}

		ogTitle := extractRomajiFromTitle(newsContent.Title)

		if err := TryReplaceTitle(&newsContent); err != nil {
			slog.Error("replace romaji title", "error", err)
		}

		queryList := GetTitles(&newsContent, ogTitle)
		duplicateFound, err := HasDuplicates(
			&gl,
			queryList,
			lemmyData.CommunityName,
		)
		if err != nil {
			slog.Error("search for duplicates", "error", err)
			continue
		}

		if duplicateFound {
			if err := db.MarkIDAsProcessed(newsID); err != nil {
				return fmt.Errorf("mark news as processed %w", err)
			}
			slog.Info("posting omitted, marked news as processed", "newsID", newsID)
			continue
		}

		if newsContent.ImageURL != "" {
			imageData, err := DownloadImage(newsContent.ImageURL)
			if err != nil {
				slog.Error("download image", "MAL_ID", newsID, "error", err)
			} else {
				imageUrl, err := gl.UploadImage(imageData)
				if err != nil {
					slog.Error("upload image", "MAL_ID", newsID, "error", err)
				} else {
					newsContent.ImageURL = imageUrl
				}
			}
		}

		postID, err := gl.Post(
			lemmyData.CommunityName,
			"[News] "+newsContent.Title,
			newsContent.Body(),
			newsContent.ImageURL,
		)
		if err != nil {
			slog.Error("post to Lemmy", "MAL_ID", newsID, "error", err)
			continue
		}

		if err := db.SetNewsEntryPostID(newsID, postID); err != nil {
			return fmt.Errorf("save post ID %v", err)
		}

		if err := db.MarkIDAsProcessed(newsID); err != nil {
			return fmt.Errorf("mark news as processed %w", err)
		}
	}

	return nil
}

func GetTitles(nc *NewsContent, ogTitle string) []string {
	queryList := []string{}
	if nc.AnimeTitleEn != "" {
		queryList = append(queryList, nc.AnimeTitleEn)
	}

	if nc.AnimeTitleRo != "" {
		queryList = append(queryList, nc.AnimeTitleRo)
	}

	if ogTitle != nc.AnimeTitleRo {
		queryList = append(queryList, ogTitle)
	}

	return queryList
}
