package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"math/rand"
)

const (
	DEBUG = false

	dbusENV  = "DBUS_SESSION_BUS_ADDRESS"
	dbusPath = "unix:path=/run/user/%d/bus"

	sourceFlickr = "flickr"
)

type Config struct {
	Sources []*Source `json:"sources"`
	Command []string  `json:"command"`
}

type Source struct {
	Priority int8   `json:"priority"`
	URL      string `json:"url"`
	Size     string `json:"size"`
}

func getConfig() (conf *Config) {
	var filecinf string

	flag.StringVar(&filecinf, "config", "./config.json", "path to config file")
	flag.Parse()

	b, err := ioutil.ReadFile(filecinf)
	if err != nil {
		panic(err)
	}

	conf = new(Config)

	if err := json.Unmarshal(b, &conf); err != nil {
		panic(err)
	}

	return
}

func main() {
	if DEBUG {
		log.Println("wallpaper change start")
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Llongfile)

	conf := getConfig()

	source := getPhotoSource(conf)
	if source == nil {
		log.Fatalln("can't get source")
	}

	img, err := getImage(source)
	if err != nil {
		log.Fatalf("get image error: %v", err)
	}

	err = changeImage(conf, img)
	if err != nil {
		log.Fatalf("can't change image: %v", err)
	}

	if DEBUG {
		log.Println("wallpaper change complete")
	}
}

func changeImage(conf *Config, img string) error {
	command := make([]string, len(conf.Command))

	for i, c := range conf.Command {
		if strings.Contains(c, "%s") {
			c = fmt.Sprintf(c, img)
		}

		command[i] = c
	}

	if err := os.Setenv(dbusENV, fmt.Sprintf(dbusPath, os.Getuid())); err != nil {
		return err
	}

	if DEBUG {
		log.Println(command[0], command[1:])
	}

	c := exec.Command(command[0], command[1:]...) //nolint:gosec // by design

	b, err := c.CombinedOutput()
	if err != nil {
		log.Println(string(b))
		return err
	}

	return nil
}

func getImage(source *Source) (string, error) {
	resp, err := http.Get(source.URL)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s bad status code: %d", source.URL, resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	imgs := parseImages(source, b)
	if len(imgs) == 0 {
		return "", errors.New("images not found")
	}

	return getRandomImg(imgs), nil
}

func getRandomImg(imgs []string) string {
	rand.Seed(time.Now().UnixNano())
	return imgs[rand.Intn(len(imgs))]
}

func parseImages(source *Source, b []byte) []string {
	switch {
	case strings.Contains(source.URL, sourceFlickr):
		return parseImagesFlickr(source, b)
	default:
		return nil
	}
}

func parseImagesFlickr(source *Source, b []byte) []string {
	var (
		reg    = regexp.MustCompile(fmt.Sprintf(`"url":"\\/\\/([^"]+_%s.jpg)"`, source.Size))
		finded = reg.FindAllStringSubmatch(string(b), -1)
		urls   = make([]string, len(finded))
		buf    bytes.Buffer
	)

	for i, matches := range finded {
		buf.WriteString("https://")
		buf.WriteString(strings.ReplaceAll(matches[1], "\\/", "/"))

		urls[i] = buf.String()

		buf.Reset()
	}

	return urls
}

func getPhotoSource(conf *Config) *Source {
	arr := []int{}

	for i, s := range conf.Sources {
		for k := int8(0); k < s.Priority; k++ {
			arr = append(arr, i)
		}
	}

	if len(arr) == 0 {
		return nil
	}

	rand.Seed(time.Now().UnixNano())

	return conf.Sources[arr[rand.Intn(len(arr))]]
}
