package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	client := http.Client{
		Transport: nil,
		Jar:       nil,
		Timeout:   0,
	}
	// wipeFileTest()
	startTime := time.Now()
	var chillspot, spawns sync.WaitGroup
	indexChannel := make(chan int)

	start := startIndex()
	end := endIndex()
	// allComics := end - start

	chillspot.Add(1)
	go func(indexChannel chan int) {
		index := 0
		granuality := 32
		for v := range indexChannel {
			if v%granuality == 0 {
				updateSkipFile(v)
				// fmt.Printf("value of v is %v \n", v)
			}
			index++
		}
		defer chillspot.Done()
	}(indexChannel)

	spawned := 0
	spawnLimit := 10
	for i := start; i <= end; i++ {
		if skipComic(i) {
			continue
		}
		spawns.Add(1)
		go downloadComic(i, &client, indexChannel, &spawns)
		spawned++
		if spawned%spawnLimit == 0 {
			spawns.Wait()
		}
	}
	// sync for both goroutines
	spawns.Wait()
	close(indexChannel)
	chillspot.Wait()
	fmt.Println(startIndex())
	fmt.Printf("took %v", time.Since(startTime))

}
func startIndex() int {
	dir, err := os.UserHomeDir()
	errNLogger(err)
	path := dir + "\\xkcd-go-comics\\index.txt"
	file, err := os.Open(path)
	errNLogger(err)
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	index, err := strconv.Atoi(scanner.Text())
	errNLogger(err)
	return index
}

func endIndex() int {
	url := "https://xkcd.com/info.0.json"
	response, err := http.Get(url)
	errNLogger(err)
	resData, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	plaintext := string(resData)
	re, err := regexp.Compile(`(?m)"num": [\d]+`)
	errNLogger(err)
	matches := re.FindAllString(plaintext, -1)
	num, err := strconv.Atoi(matches[0][7:])
	errNLogger(err)
	return num
}

func downloadComic(x int, client *http.Client, done chan int, spawns *sync.WaitGroup) {
	defer spawns.Done()
	xstr := strconv.Itoa(x)
	url := "https://xkcd.com/" + xstr + "/info.0.json"
	response, err := client.Get(url)
	if err != nil {
		response, err = retryResponse(url, client, 3)
	}
	errNLogger(err)
	if response.StatusCode != 200 {
		fmt.Printf("response status code %v", response.StatusCode)
		fmt.Printf("Url is %v\n", url)
		os.Exit(1)
	}
	responseData, err := io.ReadAll(response.Body)
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(responseData), &jsonData)
	errNLogger(err)
	defer response.Body.Close()
	errNLogger(err)
	imgURLs := jsonData["img"]
	imgURL := fmt.Sprintf("%v", imgURLs)
	// fmt.Println(imgURL)

	names := jsonData["title"]
	name := fmt.Sprintf("%v", names)
	re := regexp.MustCompile(`(?m)\.[pjg][npi][gf]`)
	exts := re.FindAllString(imgURL, -1)
	if len(exts) == 0 {
		// fmt.Println(imgURLs)
		log.Fatal("No Matches for extension" + imgURL + strconv.Itoa(x))
	}
	ext := exts[0]

	name = name + ext
	if checkName(name) == false {
		for _, v := range "?\\/*<>|:\"" {
			name = strings.Replace(name, string(v), "", -1)
		}
	}
	fmt.Println("Download complete for comic:" + name)
	// fmt.Println(imgRes.StatusCode)
	homedir, err := os.UserHomeDir()
	errNLogger(err)
	path := homedir + "\\xkcd-go-comics\\" + name
	if fileExists(path) {
		done <- x
	} else {
		imgFile, err := os.Create(path)
		errNLogger(err)

		imgRes, err := client.Get(imgURL)
		if err != nil {
			imgRes, err = retryResponse(imgURL, client, 3)
		}
		errNLogger(err)
		if imgRes.StatusCode != 200 {
			fmt.Printf("response status code %v\n", imgRes.StatusCode)
			fmt.Printf("url is %v\n", url)
			fmt.Printf("imgUrl is %v\n", imgURL)
			os.Exit(1)
		}
		imgData, err := io.ReadAll(imgRes.Body)
		defer imgRes.Body.Close()
		errNLogger(err)
		imgFile.Write(imgData)
		imgFile.Close()
		done <- x
	}
}
func errNLogger(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func skipComic(num int) bool {
	if num == 404 {
		// skipped for 'obvious reasons'
		return true
	}
	if num == 1350 || num == 1608 || num == 2198 {
		fmt.Println("Visit https://xkcd.com/" + strconv.Itoa(num) + "/ it's an interactive comic")
		return true
	}
	if num == 1037 || num == 1663 {
		fmt.Println("Skipped https://xkcd.com" + strconv.Itoa(num) + "/ the comic is not an image")
		return true
	}
	if num == 472 {
		// Randall Monroe refuses to fix the json data
		return true
	}
	return false
}
func checkName(name string) bool {
	badChars := "?\\/*<>|:\""
	for _, char := range badChars {
		if strings.Contains(name, string(char)) {
			return false
		}
	}
	return true
}

func updateSkipFile(num int) bool {
	dir, err := os.UserHomeDir()
	errNLogger(err)
	path := dir + "\\xkcd-go-comics\\index.txt"
	file, err := os.Open(path)
	defer file.Close()
	errNLogger(err)
	scanner := bufio.NewScanner(file)
	errNLogger(err)
	scanner.Scan()
	currentIndex, err := strconv.Atoi(scanner.Text())
	errNLogger(err)

	if num > currentIndex {
		file, err := os.Create(path)
		errNLogger(err)
		file.WriteString(strconv.Itoa(num))
		fmt.Printf("Updated skipFile to %v \n", num)
		return true
	}
	return false
}
func retryResponse(url string, client *http.Client, retries int) (*http.Response, error) {
	err := errors.New("Bruh")
	var response *http.Response
	for ; retries > 0 && err != nil; retries-- {
		response, err = client.Get(url)
	}
	return response, err
}
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
func offlineTest(i int, done chan int, spawns *sync.WaitGroup) {
	defer spawns.Done()
	done <- i
}
func wipeFileTest() {
	dir, err := os.UserHomeDir()
	errNLogger(err)
	path := dir + "\\xkcd-go-comics\\index.txt"
	file, err := os.Create(path)
	defer file.Close()
	file.WriteString(strconv.Itoa(1))
}
