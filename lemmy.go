package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"go.elara.ws/go-lemmy/v3"
)

type GoLemmy struct {
	Ctx      context.Context
	Client   *lemmy.Client
	ActorID  string
	ImageUrl string
}

func (g *GoLemmy) Login(instance string, username string, password string) error {
	var err error
	g.Ctx = context.Background()

	g.Client, err = lemmy.New(instance)
	if err != nil {
		return err
	}

	g.ActorID = strings.TrimSuffix(instance, "/") + "/u/" + username
	g.ImageUrl = strings.TrimSuffix(instance, "/") + "/pictrs/image"

	err = g.Client.ClientLogin(g.Ctx, lemmy.Login{
		UsernameOrEmail: username,
		Password:        password,
	})
	if err != nil {
		return err
	}

	return nil
}

type SearchResult struct {
	Name string
	Date time.Time
}

func (g *GoLemmy) Search(communityName string, queryList []string, limit int) ([]SearchResult, error) {
	var results []SearchResult
	for _, query := range queryList {
		sr, err := g.Client.Search(g.Ctx, lemmy.Search{
			CommunityName: lemmy.NewOptional(communityName),
			Q:             query,
			Type:          lemmy.NewOptional(lemmy.SearchTypePosts),
			Sort:          lemmy.NewOptional(lemmy.SortTypeNew),
			Limit:         lemmy.NewOptional(int64(limit)),
		})
		if err != nil {
			return []SearchResult{}, err
		}

		for _, post := range sr.Posts {
			// Ignore self-posts and episode threads
			if strings.ToLower(post.Creator.ActorID) != strings.ToLower(g.ActorID) &&
				!strings.Contains(post.Post.Name, "[Episode]") {
				searchResult := SearchResult{
					Name: post.Post.Name,
					Date: post.Post.Published,
				}
				results = append(results, searchResult)
			}
		}
	}
	return results, nil
}

func (g *GoLemmy) Post(communityName string, name string, body string, url string) (int64, error) {
	gcr, err := g.Client.Community(g.Ctx, lemmy.GetCommunity{
		Name: lemmy.NewOptional(communityName),
	})
	if err != nil {
		return 0, err
	}

	pr, err := g.Client.CreatePost(g.Ctx, lemmy.CreatePost{
		CommunityID: gcr.CommunityView.Community.ID,
		Name:        name,
		Body:        lemmy.NewOptional(body),
		URL:         lemmy.NewOptional(url),
	})
	if err != nil {
		return 0, err
	}

	return pr.PostView.Post.ID, nil
}

type PictrsUploadResponse struct {
	Files []PictrsFile `json:"files"`
	Msg   string       `json:"msg"`
}

type PictrsFile struct {
	DeleteToken string `json:"delete_token"`
	File        string `json:"file"`
}

func (g *GoLemmy) UploadImage(data []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("images[]", "image.jpg")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, g.ImageUrl, &body)
	if err != nil {
		return "", err
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+g.Client.Token)
	req.Header.Add("Content-Type", writer.FormDataContentType())
	req.Header.Add("User-Agent", userAgent)

	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("upload failed: %s", res.Status)
	}

	var jsonData PictrsUploadResponse
	if err := json.NewDecoder(res.Body).Decode(&jsonData); err != nil {
		return "", err
	}

	if len(jsonData.Files) == 0 {
		return "", fmt.Errorf("upload failed: response contained no files")
	}

	return g.ImageUrl + "/" + jsonData.Files[0].File, nil
}
