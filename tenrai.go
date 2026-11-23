package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// News

type Response struct {
	Data []NewsItem `json:"data"`
}

type NewsItem struct {
	MALID int64     `json:"mal_id"`
	URL   string    `json:"url"`
	Title string    `json:"title"`
	Date  time.Time `json:"date"`
}

// News Details

type ResponseDetails struct {
	Data Details `json:"data"`
}

type Details struct {
	MALID     int       `json:"mal_id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Date      time.Time `json:"date"`
	Images    Images    `json:"images"`
	Content   Content   `json:"content"`
	Relations Relations `json:"relations"`
}

type Images struct {
	JPG Image `json:"jpg"`
}

type Image struct {
	ImageURL string `json:"image_url"`
}

type Content struct {
	Markdown string `json:"markdown"`
}

type Relations struct {
	Anime []RelatedItem `json:"anime"`
	Manga []RelatedItem `json:"manga"`
}

type RelatedItem struct {
	MALID int    `json:"mal_id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type NewsContent struct {
	Title         string
	AnimeTitleEn  string
	AnimeTitleRo  string
	ImageURL      string
	Intro         string
	ReleaseDate   string
	InlineRoles   []string
	Cast          string
	Staff         string
	Synopsis      string
	TeaserPV      string
	OfficialLinks string
	Source        string
	Relations     Relations
}

func (nc *NewsContent) Body() string {
	var b strings.Builder
	if nc.ReleaseDate != "" {
		b.WriteString(nc.ReleaseDate)
		b.WriteString("  \n  \n")
	}

	if len(nc.InlineRoles) > 0 {
		for _, inlineRole := range nc.InlineRoles {
			b.WriteString(inlineRole)
			b.WriteString("  \n  \n")
		}
	}

	if nc.Cast != "" {
		b.WriteString(nc.Cast)
		b.WriteString("  \n  \n")
	}

	if nc.Staff != "" {
		b.WriteString(nc.Staff)
		b.WriteString("  \n  \n")
	}

	if nc.Synopsis != "" {
		b.WriteString("::: spoiler **Synopsis:**  \n")
		b.WriteString(nc.Synopsis)
		b.WriteString("  \n:::  \n  \n")
	}

	if nc.TeaserPV != "" {
		b.WriteString(nc.TeaserPV)
		b.WriteString("  \n  \n")
	}

	if nc.OfficialLinks != "" {
		b.WriteString(nc.OfficialLinks)
		b.WriteString("  \n  \n")
	}

	if nc.Source != "" {
		b.WriteString(nc.Source)
		b.WriteString("  \n  \n")
	}

	return strings.TrimSpace(b.String())
}

func (nc *NewsContent) GetTitles() []string {
	queryList := []string{}
	if nc.AnimeTitleEn != "" {
		queryList = append(queryList, nc.AnimeTitleEn)
	}

	if nc.AnimeTitleRo != "" {
		queryList = append(queryList, nc.AnimeTitleRo)
	}

	return queryList
}

// Replace Romaji anime title with English one within news title
func (nc *NewsContent) SwapTitles() {
	title := nc.ExtractRomajiFromTitle()
	if title != "" {
		nc.Title = strings.Replace(nc.Title, title, nc.AnimeTitleEn, 1)
	}
}

var titleFromIntro = regexp.MustCompile(`\[_([^]]+)_\]\(https://myanimelist\.net/(?:manga|anime)/\d+/?\)\s\(_([^)_]+)_\)`)

// Extract title from sentence in the first paragraph
// [_Romaji Title_](https://myanimelist.net/manga/123) (_English Title_)
func (nc *NewsContent) ExtractEnglishFromIntro() (string, string) {
	matches := titleFromIntro.FindStringSubmatch(nc.Intro)

	if len(matches) >= 3 {
		romajiTitle := matches[1]
		englishTitle := matches[2]
		return romajiTitle, englishTitle
	}

	return "", ""
}

var titleFromTitle = regexp.MustCompile(`'(.*?)'`)

// Extract title from news title 'Romaji Title'
func (nc *NewsContent) ExtractRomajiFromTitle() string {
	matches := titleFromTitle.FindStringSubmatch(nc.Title)

	if len(matches) >= 2 {
		romajiTitle := matches[1]
		return romajiTitle
	}

	return ""
}

// Try to replace Romaji anime title within news title with English anime title
func (nc *NewsContent) TryReplaceTitle() error {
	romajiTitle, englishTitle := nc.ExtractEnglishFromIntro()
	if englishTitle != "" {
		nc.AnimeTitleEn = englishTitle
		// Keep Romaji title for duplicate search
		nc.AnimeTitleRo = romajiTitle
		nc.SwapTitles()
		return nil
	}

	nc.AnimeTitleRo = nc.ExtractRomajiFromTitle()

	if nc.AnimeTitleRo != "" {
		time.Sleep(time.Second)
		found, err := nc.FindTitle("anime")
		if err != nil {
			return fmt.Errorf("find remote title %w", err)
		}
		if !found {
			_, err := nc.FindTitle("manga")
			if err != nil {
				return fmt.Errorf("find remote title %w", err)
			}
		}
	}

	return nil
}

// If romaji title is the same as one found in relations,
// try to grab English title from anime/manga details
func (nc *NewsContent) FindTitle(mediaType string) (bool, error) {
	var collection []RelatedItem
	if mediaType == "anime" {
		collection = nc.Relations.Anime
	} else {
		collection = nc.Relations.Manga
	}

	for _, content := range collection {
		if nc.AnimeTitleRo == content.Name {
			animeTitleEn, err := FetchRemoteTitle(mediaType, content.MALID)
			if err != nil {
				return false, err
			}
			if animeTitleEn != "" {
				nc.AnimeTitleEn = animeTitleEn
				nc.SwapTitles()
				return true, nil
			}

			return false, nil
		}
	}

	return false, nil
}

// Fetch English title from anime/manga details page
func FetchRemoteTitle(mediumType string, id int) (string, error) {
	url := fmt.Sprintf("https://api.tenrai.org/v1/%s/%d", mediumType, id)
	data, err := Request(url)
	if err != nil {
		return "", fmt.Errorf("download details for %s with id %d: %w", mediumType, id, err)
	}

	var jsonData AnimeResponse
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return "", fmt.Errorf("unmarshal details for %s with id %d: %w", mediumType, id, err)
	}

	return jsonData.Data.TitleEnglish, nil
}

var inlineKeywords = []string{
	"is directing",
	"directs at",
	"returns to direct",
	"is returning to direct",
	"is helming the anime",
	"assistant director",
	"voice actor",
	"voice actress",
	"cast members",
	"voicing",
	"starring as",
	"joining the cast",
	"performing the opening",
	"perform the opening",
	"performing the ending",
	"perform the ending",
	"performing the theme song",
	"perform the theme song",
	"composing the music",
	"music composer",
	"designing the characters",
	"character design",
	"sound director",
	"art director",
	"animation director",
	"animation producer",
	"series composer",
	"series composition",
}

var synopsisDelimiters = regexp.MustCompile(`(?i)(\(Source:.*?\)|\\\[.*?MAL Rewrite\\\])`)

func ParseTenraiNewsDetails(data []byte) (NewsContent, error) {

	var jsonData ResponseDetails

	if err := json.Unmarshal(data, &jsonData); err != nil {
		return NewsContent{}, err
	}

	textArray := strings.Split(jsonData.Data.Content.Markdown, "\n\n")

	var newsContent NewsContent
	newsContent.Title = jsonData.Data.Title
	newsContent.ImageURL = jsonData.Data.Images.JPG.ImageURL
	newsContent.Relations = jsonData.Data.Relations
	if len(textArray) >= 2 {
		releaseDate := getReleaseDate(textArray[1], jsonData.Data.Date)
		if releaseDate != "" {
			newsContent.ReleaseDate = "**Release date:** " + releaseDate
		}
		newsContent.Intro = textArray[1]
	}

	if len(textArray) > 2 {
		synopsisSpotted := false

		for _, paragraph := range textArray[2:] {
			clearedPara := removePreviousRoles(paragraph)

			switch {
			case strings.HasPrefix(clearedPara, "Staff  \n"):
				newsContent.Staff = strings.Replace(clearedPara, "Staff", "**Staff:**", 1)
				newsContent.Staff = strings.ReplaceAll(newsContent.Staff, " **NEW**", "")
			case strings.HasPrefix(clearedPara, "Cast  \n"):
				newsContent.Cast = strings.Replace(clearedPara, "Cast", "**Cast:**", 1)
				newsContent.Cast = strings.ReplaceAll(newsContent.Cast, " **NEW**", "")
			case strings.HasPrefix(clearedPara, "Synopsis  \n"):
				// Set a flag if synopsis could continue in other paragraphs
				if loc := synopsisDelimiters.FindStringIndex(clearedPara); loc != nil {
					clearedPara = clearedPara[:loc[0]] + clearedPara[loc[1]:]
				} else {
					synopsisSpotted = true
				}

				// Remove here, restore when printing in order to put the whole thing in spoiler
				newsContent.Synopsis = strings.Replace(clearedPara, "Synopsis  \n", "", 1)
			case strings.HasPrefix(clearedPara, "Official site: "):
				newsContent.OfficialLinks = clearedPara
			case strings.HasPrefix(clearedPara, "Source: "):
				newsContent.Source = strings.Replace(clearedPara, "Source:", "**Source:**", 1)
				newsContent.Source += fmt.Sprintf(", [MyAnimeList](%s)", jsonData.Data.URL)
				synopsisSpotted = false
			case strings.HasPrefix(clearedPara, "[YouTube video]"):
				newsContent.TeaserPV = "**PV:** " + clearedPara
			case strings.HasPrefix(clearedPara, "_Note:") ||
				strings.HasPrefix(clearedPara, "Note:"):
				continue
			default:
				if synopsisSpotted {
					if loc := synopsisDelimiters.FindStringIndex(clearedPara); loc != nil {
						clearedPara = clearedPara[:loc[0]] + clearedPara[loc[1]:]
						synopsisSpotted = false
					}

					if strings.Contains(clearedPara, "on MAL") {
						synopsisSpotted = false
						continue
					}

					newsContent.Synopsis += ("  \n" + clearedPara)
				} else if ContainsAny(clearedPara, inlineKeywords) {
					newsContent.InlineRoles = append(newsContent.InlineRoles, clearedPara)
				}
			}
		}
	}

	return newsContent, nil
}

var dateRegex = regexp.MustCompile(`(?P<prefix>broadcast|stream|debut|\sair|scheduled|premiered?).*?(?:` +
	`(?:(?P<month>January|February|March|April|May|June|July|August|September|October|November|December)\s(?P<day>\d{1,2}),\s(?P<year>\d{4}))|` +
	`(?:(?P<month>January|February|March|April|May|June|July|August|September|October|November|December)\s(?P<year>\d{4}))|` +
	`(?:(?P<month>January|February|March|April|May|June|July|August|September|October|November|December)\s(?P<day>\d{1,2}))|` +
	`(?P<month>January|February|March|April|May|June|July|August|September|October|November|December)|` +
	`(?P<year>\d{4})` +
	`)`)

var months = map[string]int{
	"January":   1,
	"February":  2,
	"March":     3,
	"April":     4,
	"May":       5,
	"June":      6,
	"July":      7,
	"August":    8,
	"September": 9,
	"October":   10,
	"November":  11,
	"December":  12,
}

func getReleaseDate(text string, publishedDate time.Time) string {
	m := dateRegex.FindStringSubmatch(text)
	if m == nil {
		return ""
	}

	fields := make(map[string]string)
	for i, name := range dateRegex.SubexpNames() {
		if name != "" && m[i] != "" {
			fields[name] = m[i]
		}
	}

	// Already aired/no match
	if fields["prefix"] == "premiered" {
		return ""
	}

	day, month, year := fields["day"], fields["month"], fields["year"]

	if month == "" {
		return year
	}

	if year == "" {
		currentYear := publishedDate.Year()
		monthNumber := months[month]

		switch {
		case monthNumber < int(publishedDate.Month()):
			currentYear++
		case monthNumber == int(publishedDate.Month()) && day != "":
			d, err := strconv.Atoi(day)
			if err == nil && d < publishedDate.Day() {
				currentYear++
			}
		}

		year = strconv.Itoa(currentYear)
	}

	if day == "" {
		return month + " " + year
	}

	return fmt.Sprintf("%s %s, %s", month, day, year)
}

var prevRole = regexp.MustCompile(`\s(\(_[^)]+\))`)

// Remove example roles written in parentheses
func removePreviousRoles(text string) string {
	return prevRole.ReplaceAllString(text, "")
}

func ParseTenraiNews(data []byte) ([]NewsItem, error) {
	var jsonData Response

	if err := json.Unmarshal(data, &jsonData); err != nil {
		return []NewsItem{}, err
	}

	sort.Slice(jsonData.Data, func(i, j int) bool {
		return jsonData.Data[i].Date.Before(jsonData.Data[j].Date)
	})

	return jsonData.Data, nil
}

var announcementKeywords = []string{
	"PV",
	"KV",
	"Visual",
	"Announced",
	"Announcement",
	"Trailer",
	"Teaser",
	"Unveils",
	"Reveals",
	"Announces",
	"Gets TV Anime",
	"Receives TV Anime",
	"Gets Season",
	"Gets Prequel",
	"Gets Sequel",
	"Gets Theatrical Anime",
	"in Production",
}

func HasDuplicates(gl *GoLemmy, queryList []string, communityName string) (bool, error) {
	if len(queryList) > 0 {
		searchResults, err := gl.Search(communityName, queryList, 20)
		if err != nil {
			return false, err
		}

		oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)
		for _, searchResult := range searchResults {
			// Ignore search results older than a week
			if searchResult.Date.Before(oneWeekAgo) {
				continue
			}
			// Look for titles and keywords
			if ContainsAny(searchResult.Name, queryList) &&
				ContainsAny(searchResult.Name, announcementKeywords) {
				slog.Info("duplicate post found", "title", searchResult.Name)
				return true, nil
			}
		}
	}

	return false, nil
}

type AnimeResponse struct {
	Data Anime `json:"data"`
}

type Anime struct {
	TitleEnglish string `json:"title_english"`
}

func DownloadNews(tag string, publishedAfter time.Time) ([]byte, error) {
	timeFormatted := publishedAfter.Format("2006-01-02T15:04:05-07:00")
	values := url.Values{}
	values.Add("tag", tag)
	values.Add("published_after", timeFormatted)
	values.Add("limit", "10")
	requestUrl := fmt.Sprintf(
		"https://api.tenrai.org/v1/news?%s",
		values.Encode(),
	)

	result, err := Request(requestUrl)
	if err != nil {
		return []byte{}, err
	}

	return result, nil
}

func DownloadNewsDetails(id int64) ([]byte, error) {
	requestUrl := fmt.Sprintf("https://api.tenrai.org/v1/news/%d", id)

	result, err := Request(requestUrl)
	if err != nil {
		return []byte{}, err
	}

	return result, nil
}
