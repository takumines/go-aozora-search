package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/text/encoding/japanese"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type Entry struct {
	AuthorID string
	Author   string
	TitleID  string
	Title    string
	SiteURL  string
	ZipURL   string
}

func setupDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS authors(author_id TEXT, author TEXT, PRIMARY KEY (author_id))`,
		`CREATE TABLE IF NOT EXISTS contents(author_id TEXT, title_id TEXT, title TEXT, content TEXT, PRIMARY KEY (author_id, title_id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS contents_fts USING fts4(words)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			log.Fatal(err)
		}
	}

	return db, nil
}

func addEntry(db *sql.DB, entry *Entry, content string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO authors(author_id, author) VALUES(?, ?)`, entry.AuthorID, entry.Author)
	if err != nil {
		return err
	}
	res, err := db.Exec(`INSERT OR REPLACE INTO contents(author_id, title_id, title, content) VALUES(?, ?, ?, ?)`, entry.AuthorID, entry.TitleID, entry.Title, content)
	if err != nil {
		return err
	}

	docID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return err
	}
	seg := t.Wakati(content)
	_, err = db.Exec(`REPLACE INTO contents_fts(docid, words) VALUES(?, ?)`, docID, strings.Join(seg, " "))
	if err != nil {
		return err
	}

	return nil
}

func findEntries(siteURL string) ([]Entry, error) {
	resp, err := http.Get(siteURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var entries []Entry
	pat := regexp.MustCompile(`.*/cards/([0-9]+)/card([0-9]+).html$`)
	doc.Find("ol li  a").Each(func(i int, elem *goquery.Selection) {
		token := pat.FindStringSubmatch(elem.AttrOr("href", ""))
		if len(token) != 3 {
			return
		}
		title := elem.Text()
		pageURL := fmt.Sprintf("https://www.aozora.gr.jp/cards/%s/card%s.html",
			token[1], token[2])
		author, zipURL := findAuthorAndZIP(pageURL)
		if zipURL != "" {
			entries = append(entries, Entry{
				AuthorID: token[1],
				Author:   author,
				TitleID:  token[2],
				Title:    title,
				SiteURL:  siteURL,
				ZipURL:   zipURL,
			})
		}
	})

	return entries, nil
}

func findAuthorAndZIP(siteURL string) (string, string) {
	resp, err := http.Get(siteURL)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", ""
	}
	author := doc.Find("table[summary=作家データ] tr:nth-child(2) td:nth-child(2)").Text()

	zipURL := ""
	doc.Find("table.download a").Each(func(i int, elem *goquery.Selection) {
		href := elem.AttrOr("href", "")
		if strings.HasSuffix(href, ".zip") {
			zipURL = href
		}
	})

	if zipURL == "" {
		return author, ""
	}

	if strings.HasPrefix(zipURL, "http://") || strings.HasPrefix(zipURL, "https://") {
		return author, zipURL
	}

	u, err := url.Parse(siteURL)
	if err != nil {
		return author, ""
	}
	// zipURL (./files/171_ruby_1273.zip)からURLを生成する(https://www.aozora.gr.jp/cards/000879/files/171_ruby_1273.zip)
	u.Path = path.Join(path.Dir(u.Path), zipURL)

	return author, u.String()

	return author, zipURL
}

func extractText(zipURL string) (string, error) {
	resp, err := http.Get(zipURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// zipファイルの内容読み込み
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	// 読み込んだファイルの中から拡張子が.txtのファイルを探す
	for _, file := range r.File {
		if path.Ext(file.Name) == ".txt" {
			// 見つかったらファイルの内容を読み込む
			f, err := file.Open()
			if err != nil {
				return "", err
			}
			b, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return "", err
			}
			// ShiftJISからUTF-8に変換する
			b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}

	return "", errors.New("content not found")
}

func main() {
	db, err := setupDB("../database.sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"

	entries, err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("found %d entries", len(entries))
	for _, entry := range entries {
		log.Printf("adding %+v\n", entry)
		content, err := extractText(entry.ZipURL)
		if err != nil {
			log.Println(err)
			continue
		}
		err = addEntry(db, &entry, content)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}
