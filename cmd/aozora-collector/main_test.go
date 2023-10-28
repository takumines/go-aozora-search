package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
)

func TestFindEntries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.String())

		if r.URL.String() == "/" {
			w.Write([]byte(`
			<table summary="作家データ">
				<tbody>
					<tr><td class="header">分類：</td><td>著者</td></tr>
					<tr><td class="header">作家名：</td><td><font size="+2">テスト 太郎</font></td></tr>
					<tr><td class="header">作家名読み：</td><td>てすと たろう</td></tr>
					<tr><td class="header">ローマ字表記：</td><td>Test, Taro</td></tr>
				</tbody>
			</table>
			<ol>
				<li><a href="/cards/999999/card000001.html">テスト書籍001</a></li>
				<li><a href="/cards/999999/card000002.html">テスト書籍002</a></li>
				<li><a href="/cards/999999/card000003.html">テスト書籍003</a></li>
			</ol>
			`))
		} else {
			pat := regexp.MustCompile(`.*/cards/([0-9]+)/card([0-9]+).html$`)
			token := pat.FindStringSubmatch(r.URL.String())
			w.Write([]byte(fmt.Sprintf(`
			<table summary="作家データ">
			<tbody>
				<tr><td class="header">分類：</td><td>著者</td></tr>
				<tr><td class="header">作家名：</td><td><font size="+2">テスト 太郎</font></td></tr>
				<tr><td class="header">作家名読み：</td><td>てすと たろう</td></tr>
				<tr><td class="header">ローマ字表記：</td><td>Test, Taro</td></tr>
			</tbody>
			</table>
			<table border="1" summary="ダウンロードデータ" class="download">
				<tbody>
					<tr><td><a href="./files/%[1]s_%[2]s.zip">%[1]s_%[2]s.zip</a><td></tr>
				</tbody>
			</table>
			`, token[1], token[2])))
		}
	}))
	defer ts.Close()

	tmp := pageURLFormat
	pageURLFormat = ts.URL + "/cards/%s/card%s.html"
	defer func() {
		// テスト終了後に初期値に戻す
		pageURLFormat = tmp
	}()

	got, err := findEntries(ts.URL)
	if err != nil {
		t.Error(err)
		return
	}

	want := []Entry{
		{
			Author:   "テスト 太郎",
			AuthorID: "999999",
			Title:    "テスト書籍001",
			TitleID:  "000001",
			SiteURL:  ts.URL,
			ZipURL:   ts.URL + "/cards/999999/files/999999_000001.zip",
		},
		{
			Author:   "テスト 太郎",
			AuthorID: "999999",
			Title:    "テスト書籍002",
			TitleID:  "000002",
			SiteURL:  ts.URL,
			ZipURL:   ts.URL + "/cards/999999/files/999999_000002.zip",
		},
		{
			Author:   "テスト 太郎",
			AuthorID: "999999",
			Title:    "テスト書籍003",
			TitleID:  "000003",
			SiteURL:  ts.URL,
			ZipURL:   ts.URL + "/cards/999999/files/999999_000003.zip",
		},
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("want: %+v, got: %+v", want, got)
	}
}

func TextExtractText(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer ts.Close()

	got, err := extractText(ts.URL + "/testdata/example.zip")
	if err != nil {
		t.Fatal(err)
		return
	}

	want := "テストデータ\n"
	if want != got {
		t.Errorf("want: %s, got: %s", want, got)
	}
}
