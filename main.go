package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/bitly/go-simplejson"
	"github.com/PuerkitoBio/goquery"
)

type bookData struct {
	Title  string `json:"title"`
	Isbn10 string `json:"isbn_10,omitempty"`
	Isbn13 string `json:"isbn_13,omitempty"`
	Other  string `json:"other,omitempty"`
}

type bookList struct {
	BookList []bookData `json:"want_to_read"`
}

func getBookTitle(uid string) (titleList []string) {
	bookMeterURL := fmt.Sprintf("https://bookmeter.com/users/%s/books/wish", uid)

	req, err := http.NewRequest("GET", bookMeterURL, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Accept-encoding", "gzip")

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			panic(err)
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		panic(err)
	}

	doc.Find("img.cover__image").Each(func(index int, s *goquery.Selection) {
		title, exist := s.Attr("alt")
		if exist {
			titleList = append(titleList, title)
		}
	})

	return
}

func getIsbn(titleList []string, routineSize int, verbose bool) (list bookList) {
	const GoogleBooksAPI = "https://www.googleapis.com/books/v1/volumes?q="

	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	ch := make(chan bool, routineSize)

	dataList := []bookData{}
	for _, title := range titleList {
		wg.Add(1)

		go func(t string) {
			ch <- true
			defer func() { <-ch }()
			defer wg.Done()

			if verbose {
				fmt.Printf("Get %s ISBN...\n", t)
			}

			resp, err := http.Get(GoogleBooksAPI + url.QueryEscape(t))
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			byteArray, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}

			jsonData, err := simplejson.NewJson(([]byte)(byteArray))
			if err != nil {
				panic(err)
			}

			data := bookData{Title: t}
			idTypeMap := map[string]*string{
				"ISBN_10": &data.Isbn10,
				"ISBN_13": &data.Isbn13,
				"OTHER":   &data.Other,
			}
			industoryIDs := jsonData.Get("items").GetIndex(0).GetPath("volumeInfo", "industryIdentifiers").MustArray()
			for _, IDIf := range industoryIDs {
				ID := IDIf.(map[string]interface{})
				idtype, _ := ID["type"].(string)
				isbn, _ := ID["identifier"].(string)
				*idTypeMap[idtype] = isbn
			}

			mutex.Lock()
			dataList = append(dataList, data)
			mutex.Unlock()
		}(title)
	}
	wg.Wait()

	list.BookList = dataList
	return
}

func main() {
	var (
		userID       = flag.String("u", "XXXXXX", "UsesrID for book-meter.")
		routineLimit = flag.Int("l", 3, "Limmit of the goroutine num.")
		outPath      = flag.String("o", "./book_list.json", "Output path for Json bool list.")
		verbose      = flag.Bool("v", false, "Redundant stdout ")
	)
	flag.Parse()

	titleList := getBookTitle(*userID)
	bookList := getIsbn(titleList, *routineLimit, *verbose)

	json, err := json.MarshalIndent(bookList, "", "    ")
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile(*outPath, json, os.ModePerm)
}
