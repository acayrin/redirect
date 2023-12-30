package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type AppConfig struct {
	ServiceListURL string
	Interval       int
}

type ServiceType struct {
	Type      string
	TestURL   string
	Fallback  string
	Instances []string
}

type ServiceState struct {
	Instance  string
	Fallback  string
	Timestamp int64
	Working   bool
}

var lastChecked int64
var serviceStates map[string][]ServiceState

func load() {
	resp, err := http.Get("https://raw.githubusercontent.com/benbusby/farside/main/services-full.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	var serviceList []ServiceType
	err = json.Unmarshal(body, &serviceList)
	if err != nil {
		fmt.Println(err)
		return
	}

	serviceStates = make(map[string][]ServiceState)

	for _, service := range serviceList {
		serviceStates[service.Type] = []ServiceState{}

		go func(service ServiceType) {
			for _, instance := range service.Instances {
				for _, subInstance := range strings.Split(instance, "|") {
					if strings.HasSuffix(subInstance, ".onion") {
						continue
					}

					serviceStates[service.Type] = append(serviceStates[service.Type], ServiceState{
						Instance:  subInstance,
						Fallback:  service.Fallback,
						Timestamp: time.Now().Unix(),
						Working:   checkWorking(subInstance, service.TestURL),
					})
				}
			}

			fmt.Printf("[%s] %d/%d\n", service.Type, len(serviceStates[service.Type]), len(service.Instances))
		}(service)
	}

	lastChecked = time.Now().Unix()
}

func checkWorking(instance string, testURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get(instance + testURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200
}

func main() {
	go load()
	go func() {
		for range time.Tick(time.Duration(300) * time.Second) {
			load()
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		url := r.URL
		urlService := strings.Split(url.Path[1:], "/")[0]
		urlPath := url.Path[len(urlService)+1:]

		if serviceStates[urlService] == nil {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<div><i>Last checked: %s</i></div>", time.Unix(lastChecked, 0).UTC().Format(time.RFC1123Z))
			for key := range serviceStates {
				fmt.Fprintf(w, "<h2>%s</h2>", key)
				for _, state := range serviceStates[key] {
					color := "red"
					if state.Working {
						color = "blue"
					}
					fmt.Fprintf(w, "<div><a style=\"color:%s\" href=\"%s\"><b>%s</b></a></div>", color, state.Instance, state.Instance)
				}
			}
			return
		}

		service := serviceStates[urlService]
		workingServices := make([]ServiceState, 0)
		for _, state := range service {
			if state.Working {
				workingServices = append(workingServices, state)
			}
		}

		randomIndex := rand.Intn(len(workingServices))
		selectedService := workingServices[randomIndex]

		http.Redirect(w, r, selectedService.Instance+urlPath, http.StatusFound)
	})

	http.ListenAndServe(":8000", nil)
}
