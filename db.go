package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

type SQLiteDB struct {
	Ctx context.Context
	DB  *sql.DB
}

func (sdb *SQLiteDB) Init(driver string, path string) error {
	var err error
	sdb.Ctx = context.Background()
	sdb.DB, err = sql.Open(driver, path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := sdb.DB.PingContext(sdb.Ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	// Create table for processed news
	_, err = sdb.DB.ExecContext(sdb.Ctx, `
		CREATE TABLE IF NOT EXISTS news_items (
			id             INTEGER PRIMARY KEY,
			title          TEXT NOT NULL,
			lemmy_id       INTEGER NULL,
			published_at   INTEGER NOT NULL,
			check_keywords BOOLEAN NOT NULL DEFAULT FALSE,
			processed      BOOLEAN NOT NULL DEFAULT FALSE
		)`,
	)
	if err != nil {
		return fmt.Errorf("create news_items table: %w", err)
	}

	// Create table for recording the time of the last update for each tag
	_, err = sdb.DB.ExecContext(sdb.Ctx, `
		CREATE TABLE IF NOT EXISTS tag_state (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			last_update INTEGER NOT NULL,
			tag         TEXT UNIQUE NOT NULL
		)`,
	)
	if err != nil {
		return fmt.Errorf("create tag_state table: %w", err)
	}

	var hasEntries bool
	err = sdb.DB.QueryRowContext(
		sdb.Ctx,
		`SELECT EXISTS (SELECT 1 FROM tag_state)`,
	).Scan(&hasEntries)
	if err != nil {
		return fmt.Errorf("check tag_state: %w", err)
	}

	if !hasEntries {
		// Insert followed tags on the first run
		_, err = sdb.DB.ExecContext(sdb.Ctx,
			`INSERT INTO tag_state (last_update, tag) VALUES (?, ?), (?, ?)`,
			time.Now().UTC().Unix(), "New Anime",
			time.Now().UTC().Unix(), "More Info",
		)
		if err != nil {
			return fmt.Errorf("insert into tag_state on first run: %w", err)
		}
	}

	return nil
}

func (sdb *SQLiteDB) GetTagDate(tag string) (time.Time, error) {
	var lastUpdate int64
	err := sdb.DB.QueryRowContext(
		sdb.Ctx,
		`SELECT last_update FROM tag_state WHERE tag = ? LIMIT 1`,
		tag,
	).Scan(&lastUpdate)
	if err != nil {
		return time.Time{}, fmt.Errorf("get last_update for tag %s: %w", tag, err)
	}

	return time.Unix(lastUpdate, 0).UTC(), nil
}

func (sdb *SQLiteDB) UpdateTagDate(tag string) error {
	_, err := sdb.DB.ExecContext(sdb.Ctx,
		`UPDATE tag_state SET last_update = ? WHERE tag = ?`,
		time.Now().UTC().Unix(),
		tag,
	)
	if err != nil {
		return fmt.Errorf("update last_update for tag %s: %w", tag, err)
	}

	return nil
}

func (sdb *SQLiteDB) InsertNewsEntry(
	id int64,
	title string,
	publishedAt time.Time,
	checkKeywords bool,
) error {
	_, err := sdb.DB.ExecContext(sdb.Ctx, `
		INSERT INTO news_items (id, title, published_at, check_keywords)
		VALUES (?, ?, ?, ?)`,
		id,
		title,
		publishedAt.UTC().Unix(),
		checkKeywords,
	)
	if err != nil {
		return fmt.Errorf("insert news_items row %d: %w", id, err)
	}

	return nil
}

func (sdb *SQLiteDB) ExistsNewsEntry(id int64) (bool, error) {
	var hasEntry bool
	err := sdb.DB.QueryRowContext(
		sdb.Ctx,
		`SELECT EXISTS (SELECT 1 FROM news_items WHERE id = ?)`,
		id,
	).Scan(&hasEntry)
	if err != nil {
		return false, fmt.Errorf("exists ID %d: %w", id, err)
	}

	return hasEntry, nil
}

func (sdb *SQLiteDB) GetUnprocessedNewsEntryIDs() ([]int64, error) {
	rows, err := sdb.DB.QueryContext(sdb.Ctx, `
		SELECT id
		FROM news_items
		WHERE processed = FALSE
		ORDER BY published_at ASC, id ASC
		`,
	)
	if err != nil {
		return []int64{}, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return []int64{}, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return []int64{}, err
	}

	return ids, nil
}

func (sdb *SQLiteDB) MarkIDAsProcessed(id int64) error {
	_, err := sdb.DB.ExecContext(sdb.Ctx,
		`UPDATE news_items SET processed = TRUE WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("set id %d as processed: %w", id, err)
	}

	return nil
}

func (sdb *SQLiteDB) SetNewsEntryPostID(id int64, postID int64) error {
	_, err := sdb.DB.ExecContext(sdb.Ctx,
		`UPDATE news_items SET lemmy_id = ? WHERE id = ?`,
		postID,
		id,
	)
	if err != nil {
		return fmt.Errorf("set lemmy_id %d for entry with ID %d: %w", postID, id, err)
	}

	return nil
}

func (sdb *SQLiteDB) GetCheckKeywords(id int64) (bool, error) {
	var checkKeywords bool
	err := sdb.DB.QueryRowContext(
		sdb.Ctx,
		`SELECT check_keywords FROM news_items WHERE id = ?`,
		id,
	).Scan(&checkKeywords)
	if err != nil {
		return false, fmt.Errorf("get check_keywords ID %d: %w", id, err)
	}

	return checkKeywords, nil
}
