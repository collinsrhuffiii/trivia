package main

import (
	"bufio"
	"fmt"
	"github.com/go-vgo/robotgo"
	"github.com/oliamb/cutter"
	"github.com/otiai10/gosseract"
	"github.com/vova616/screenshot"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	customsearch "google.golang.org/api/customsearch/v1"
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var width int
var height int
var anchorX int
var anchorY int

func initializeView() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Move mouse to upper left corner and press enter")
	_, _ = reader.ReadString('\n')
	x1, y1 := robotgo.GetMousePos()
	fmt.Println("Move mouse to lower right corner and press enter")
	_, _ = reader.ReadString('\n')
	x2, y2 := robotgo.GetMousePos()
	anchorX = x1
	anchorY = y1
	width = x2 - x1
	height = y2 - y1
}

func getImage() {
	img, _ := screenshot.CaptureScreen()
	myImg := image.Image(img)
	toImg, _ := os.Create("original.png")
	defer toImg.Close()
	png.Encode(toImg, myImg)
	croppedImg, _ := cutter.Crop(myImg, cutter.Config{
		Width:  width,
		Height: height,
		Anchor: image.Point{anchorX, anchorY},
	})

	toImgCropped, _ := os.Create("cropped.png")
	defer toImgCropped.Close()
	png.Encode(toImgCropped, croppedImg)
}

func getText() string {
	client := gosseract.NewClient()
	defer client.Close()
	client.SetImage("cropped.png")
	text, err := client.Text()
	if err != nil {
		log.Fatal(err)
	}
	return text
}

func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}

func splitText(s string) (string, []string) {
	i := 0
	for ; s[i] != '?'; i++ {
	}
	i++
	question := s[:i]
	answers := strings.Split(s[i:], "\n")
	answers = deleteEmpty(answers)

	return strings.Replace(question, "\n", " ", -1), answers
}

func initializeSearch() (*customsearch.Service, string) {
	data, err := ioutil.ReadFile("search-key.json")
	if err != nil {
		log.Fatal(err)
	}
	//Get the config from the json key file with the correct scope
	conf, err := google.JWTConfigFromJSON(data, "https://www.googleapis.com/auth/cse")
	if err != nil {
		log.Fatal(err)
	}

	// Initiate an http.Client. The following GET request will be
	// authorized and authenticated on the behalf of
	// your service account.
	client := conf.Client(oauth2.NoContext)
	cseService, err := customsearch.New(client)
	if err != nil {
		log.Fatal(err)
	}
	idByte, err := ioutil.ReadFile("id.txt")
	if err != nil {
		log.Fatal(err)
	}
	id := string(idByte)
	id = strings.Replace(id, "\n", "", -1)
	return cseService, id
}

func countAnswersFull(results []*customsearch.Result, answers []string) []int {
	counts := make([]int, len(answers))
	ch := make(chan []int)
	for i := 0; i < 5; i++ {
		result := results[i]
		go countAnswersPage(result.Link, answers, ch)
	}
	for i := 0; i < 5; i++ {
		cur := <-ch
		for j := range answers {
			counts[j] = counts[j] + cur[j]
		}
	}
	return counts
}

func countAnswersSnippet(results []*customsearch.Result, answers []string) []int {
	counts := make([]int, len(answers))
	for _, result := range results {
		for i, answer := range answers {
			counts[i] = counts[i] + strings.Count(result.HtmlSnippet, answer)
		}
	}
	return counts
}

func countAnswersPage(url string, answers []string, ch chan<- []int) {
	counts := make([]int, len(answers))
	resp, err := http.Get(url)
	s, _ := ioutil.ReadAll(resp.Body)
	html := string(s)
	html = strings.ToLower(html)
	if err == nil {
		for i, val := range answers {
			counts[i] = strings.Count(html, val)
		}
	}
	ch <- counts
}

func isZeros(array []int) bool {
	for _, val := range array {
		if val != 0 {
			return false
		}
	}
	return true
}

/*
func questionKeywordSearch(question string, answers []string, r *rake.Rake, cseService *customsearch.Service) []int {
	wordScores := r.Run(question)
	keywordString := ""
	for _, word := range wordScores {
		keywordString = keywordString + word.Keyword
	}

	search := cseService.Cse.List(
	search.Cx(searchID)
	result, _ := search.Do()

}
*/
func search(cseService *customsearch.Service, query, searchID string) []*customsearch.Result {
	search := cseService.Cse.List(query)
	search.Cx(searchID)
	searchResults, err := search.Do()
	if err != nil {
		log.Fatal(err)
	}
	return searchResults.Items
}

func main() {
	cseService, searchID := initializeSearch()
	initializeView()
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Press enter when the question appears")
		_, _ = reader.ReadString('\n')
		start := time.Now()
		getImage()
		question, answers := splitText(getText())
		question = strings.ToLower(question)
		for i := range answers {
			answers[i] = strings.ToLower(answers[i])
		}
		fmt.Println(question)
		var bestAnswer string
		if strings.Contains(question, "not") {
			var smallest int
			question = strings.Replace(question, "not", "", 1)
			results := search(cseService, question, searchID)
			counts := countAnswersFull(results, answers)
			for i, val := range answers {
				fmt.Printf("\n%q\t\t%d", val, counts[i])
				if counts[i] <= smallest {
					smallest = counts[i]
					bestAnswer = val
				}
			}
		} else {
			var largest int
			results := search(cseService, question, searchID)
			counts := countAnswersFull(results, answers)
			for i, val := range answers {
				fmt.Printf("\n%q\t\t%d", val, counts[i])
				if counts[i] > largest {
					largest = counts[i]
					bestAnswer = val
				}
			}
		}
		fmt.Printf("\n*** Best Answer ***\n%q\n****************\n", bestAnswer)
		t := time.Now()
		elapsed := t.Sub(start)
		fmt.Println("Time elapsed: ", elapsed)
	}
}
