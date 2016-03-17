package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
)

type image struct {
	Image string                `json:"image"` //image name
	Hook  string                `json:"hook"`
	Tags  map[string][][]string `json:"tags"`
}

var (
	conf   = flag.String("conf", "", "configuration file to read from")
	addr   = flag.String("http", ":8080", "http address to listen on")
	images []image
)

func execHook(image, tag string, commands [][]string) {
	log.Printf("Executing hook for image %s:%s", image, tag)
	for _, command := range commands {
		var cmd *exec.Cmd
		if len(command) == 1 {
			cmd = exec.Command(command[0])
		} else {
			cmd = exec.Command(command[0], command[1:]...)
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("Error while executing hook for %s:%s: %v", image, tag, err)
		}
	}
}

func getCommands(imageName, tagName string) [][]string {
	for _, image := range images {
		if image.Image != imageName {
			continue
		}

		for tag, commands := range image.Tags {
			if tag == tagName {
				return commands
			}
		}
	}

	return [][]string{}
}

func hookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var data struct {
		PushData struct {
			Tag string `json:"tag"`
		} `json:"push_data"`

		Repository struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"repository"`
	}

	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if data.Repository.Namespace == "" {
		http.Error(w, "missing details", http.StatusBadRequest)
		return
	}
	if data.Repository.Name == "" {
		http.Error(w, "missing details", http.StatusBadRequest)
		return
	}

	image := data.Repository.Namespace + "/" + data.Repository.Name
	tag := data.PushData.Tag
	commands := getCommands(image, tag)
	if len(commands) == 0 {
		log.Printf("Couldn't find commands for %s:%s", image, tag)
		return
	}

	execHook(image, tag, commands)
}

func main() {
	flag.Parse()
	if *conf == "" {
		flag.PrintDefaults()
		return
	}

	bytes, err := ioutil.ReadFile(*conf)
	if err != nil {
		log.Fatal(err)
	}

	if err = json.Unmarshal(bytes, &images); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})

	for _, image := range images {
		if image.Hook == "" {
			log.Fatalf("No hook path mentioned for image %s", image.Image)
		}

		log.Printf("Registering /%s for %s", image.Hook, image.Image)
		http.HandleFunc("/"+image.Hook, hookHandler)
	}
	log.Println("Serving on", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
