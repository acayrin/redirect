package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type Service struct {
	Type      string   `json:"type"`
	TestURL   string   `json:"test_url"`
	Fallback  string   `json:"fallback"`
	Instances []string `json:"instances"`
}

var services []Service
var serviceStates map[string][]struct {
	Server    string `json:"server"`
	Available bool   `json:"available"`
}

func fetchWithTimeout(url string, timeout time.Duration) (*http.Response, error) {
	client := http.Client{
		Timeout: timeout,
	}
	return client.Head(url)
}

func fetchServiceList() error {
	resp, err := http.Get("https://raw.githubusercontent.com/benbusby/farside/main/services-full.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &services)
	if err != nil {
		return err
	}

	return nil
}

func checkServiceStates() error {
	fmt.Println("[main] Checking services...")

	for _, sv := range services {
		go func(sv Service) {
			var serviceAvailability []struct {
				Server    string `json:"server"`
				Available bool   `json:"available"`
			}

			for _, insMultiUrls := range sv.Instances {
				insMultiAvailability := true

				insMultiUrlsArr := strings.Split(insMultiUrls, "|")
				for _, insSingleUrl := range insMultiUrlsArr {
					response, err := fetchWithTimeout(fmt.Sprintf("%s/%s", insSingleUrl, sv.TestURL), 3*time.Second)
					if err != nil || strings.HasPrefix(response.Status, "5") {
						insMultiAvailability = false
						break
					}
				}

				if insMultiAvailability {
					serviceAvailability = append(serviceAvailability, struct {
						Server    string `json:"server"`
						Available bool   `json:"available"`
					}{
						Server:    insMultiUrls,
						Available: true,
					})
				} else {
					serviceAvailability = append(serviceAvailability, struct {
						Server    string `json:"server"`
						Available bool   `json:"available"`
					}{
						Server:    insMultiUrls,
						Available: false,
					})
				}
			}

			fmt.Printf("[%s] Available: %d/%d\n", sv.Type, len(serviceAvailability), len(sv.Instances))

			serviceStates[sv.Type] = serviceAvailability
		}(sv)
	}

	return nil
}

func renderEmptyResponseBody() string {
	var emptyResponseBody []string

	for svType, svStates := range serviceStates {
		var svInstances []string

		for _, ins := range svStates {
			svInstances = append(svInstances, fmt.Sprintf("- <a href=\"%s\" style=\"color:%s\">%s</a>", ins.Server, func() string {
				if ins.Available {
					return "green"
				}
				return "red"
			}(), ins.Server))
		}

		emptyResponseBody = append(emptyResponseBody, fmt.Sprintf("<h3>%s</h3>\n%s", svType, strings.Join(svInstances, "<br/>")))
	}

	return strings.Join(emptyResponseBody, "\n")
}

func main() {
	serviceStates = make(map[string][]struct {
		Server    string `json:"server"`
		Available bool   `json:"available"`
	})

	err := fetchServiceList()
	if err != nil {
		fmt.Println("Failed to fetch service list:", err)
		return
	}

	err = checkServiceStates()
	if err != nil {
		fmt.Println("Failed to check service states:", err)
		return
	}

	renderEmptyResponseBody()

	go func() {
		for range time.Tick(5 * time.Minute) {
			err := checkServiceStates()
			if err != nil {
				fmt.Println("Failed to check service states:", err)
			}
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		url := r.URL
		requestService := strings.Split(url.Path, "/")[1]
		var matchingService string

		for _, sv := range services {
			if sv.Type == requestService {
				matchingService = sv.Type
				break
			}
		}

		if matchingService != "" {
			var availableServices []struct {
				Server    string `json:"server"`
				Available bool   `json:"available"`
			}

			for _, sv := range serviceStates[matchingService] {
				if sv.Available {
					availableServices = append(availableServices, sv)
				}
			}

			if len(availableServices) > 0 {
				availableService := availableServices[rand.Intn(len(availableServices))]
				urlQueryString := url.Query().Encode()
				if len(urlQueryString) > 0 {
					urlQueryString = "?" + urlQueryString
				}
				http.Redirect(w, r, fmt.Sprintf("%s%s%s", availableService.Server, url.Path[len(matchingService)+1:], urlQueryString), http.StatusMovedPermanently)
			} else {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"status": 400, "message": "No server available to redirect"}`))
			}
		} else {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(renderEmptyResponseBody()))
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Println("Failed to start server:", err)
	}
}
