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
	"time"
)

func main() {
	client := http.Client{
		Transport: nil,
		Jar:       nil,
		Timeout:   0,
	}
	startTime := time.Now()
	initStream := make(chan int, 8)
	doneStream := make(chan int, 8)

	start := startIndex()
	end := endIndex()
	granuality := 32

	func(initStream, doneStream chan int) {
		for i := start; i < end; i++ {
			if skipComic(i) {
				go offlineTest(i, initStream, doneStream)
				continue
			}
			go downloadComic(i, &client, initStream, doneStream)
		}
		done := 0
		for v := range doneStream {
			<-initStream
			done++
			if v%granuality == 0 {
				updateSkipFile(v)
			}
			if done == (end - start) {
				close(doneStream)
			}
		}
	}(initStream, doneStream)

	fmt.Printf("took %v\n", time.Since(startTime))
}

func startIndex() int {
	dir, err := os.UserHomeDir()
	errorLogger(err)
	path := dir + "/xkcd-comics/index.txt"
	if !fileExists(path) {
		err := os.MkdirAll(dir+"/xkcd-comics", os.FileMode(0775))
		errorLogger(err)
		file, err := os.Create(path)
		errorLogger(err)
		file.WriteString("1")
		file.Close()
	}
	file, err := os.Open(path)
	errorLogger(err)
	scanner := bufio.NewScanner(file)
	scanner.Scan()
	index, err := strconv.Atoi(scanner.Text())
	errorLogger(err)
	return index
}

func endIndex() int {
	url := "https://xkcd.com/info.0.json"
	response, err := http.Get(url)
	errorLogger(err)
	resData, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	plaintext := string(resData)
	re, err := regexp.Compile(`(?m)"num": [\d]+`)
	errorLogger(err)
	matches := re.FindAllString(plaintext, -1)
	num, err := strconv.Atoi(matches[0][7:])
	errorLogger(err)
	return num
}

func downloadComic(x int, client *http.Client, initStream, doneStream chan int) {
	xstr := strconv.Itoa(x)
	url := "https://xkcd.com/" + xstr + "/info.0.json"
	response, err := client.Get(url)
	if err != nil {
		response, err = retryResponse(url, client, 3)
	}
	errorLogger(err)
	if response.StatusCode != 200 {
		fmt.Printf("response status code %v", response.StatusCode)
		fmt.Printf("Url is %v\n", url)
		os.Exit(1)
	}
	responseData, err := io.ReadAll(response.Body)
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(responseData), &jsonData)
	errorLogger(err)
	defer response.Body.Close()
	errorLogger(err)
	imgURLs := jsonData["img"]
	imgURL := fmt.Sprintf("%v", imgURLs)

	names := jsonData["title"]
	name := fmt.Sprintf("%v", names)
	re := regexp.MustCompile(`(?m)\.[pjg][npi][gf]`)
	exts := re.FindAllString(imgURL, -1)
	if len(exts) == 0 {
		log.Fatal("No Matches for extension" + imgURL + strconv.Itoa(x))
	}
	ext := exts[0]

	name = name + ext
	if checkName(name) == false {
		for _, v := range "?\\/*<>|:\"" {
			name = strings.Replace(name, string(v), "", -1)
		}
	}
	homedir, err := os.UserHomeDir()
	errorLogger(err)
	path := homedir + "/xkcd-comics/" + name
	initStream <- x
	if fileExists(path) && !fileIsEmpty(path) {
		doneStream <- x
	} else {
		imgRes, err := client.Get(imgURL)
		if err != nil {
			imgRes, err = retryResponse(imgURL, client, 3)
		}
		errorLogger(err)
		if imgRes.StatusCode != 200 {
			fmt.Printf("response status code %v\n", imgRes.StatusCode)
			fmt.Printf("url is %v\n", url)
			fmt.Printf("imgUrl is %v\n", imgURL)
			os.Exit(1)
		}
		imgData, err := io.ReadAll(imgRes.Body)
		defer imgRes.Body.Close()
		errorLogger(err)

		imgFile, err := os.Create(path)
		errorLogger(err)

		imgFile.Write(imgData)
		fmt.Println("Download complete for comic:" + name)
		imgFile.Close()
		doneStream <- x
	}
}

func errorLogger(err error) {
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
		fmt.Println("Skipped https://xkcd.com/" + strconv.Itoa(num) + "/ the comic is not an image")
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
	errorLogger(err)
	path := dir + "/xkcd-comics/index.txt"
	file, err := os.Open(path)
	defer file.Close()
	errorLogger(err)
	scanner := bufio.NewScanner(file)
	errorLogger(err)
	scanner.Scan()
	currentIndex, err := strconv.Atoi(scanner.Text())
	errorLogger(err)

	if num > currentIndex {
		file, err := os.Create(path)
		errorLogger(err)
		file.WriteString(strconv.Itoa(num))
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

func fileIsEmpty(path string) bool {
	stat, err := os.Stat(path)
	errorLogger(err)

	if stat.Size() == 0 {
		return true
	}
	return false
}

func offlineTest(i int, initStream, doneStream chan int) {
	initStream <- i
	doneStream <- i
}

func wipeFileTest() {
	dir, err := os.UserHomeDir()
	errorLogger(err)
	path := dir + "/xkcd-comics/index.txt"
	file, err := os.Create(path)
	defer file.Close()
	file.WriteString(strconv.Itoa(1))
}
