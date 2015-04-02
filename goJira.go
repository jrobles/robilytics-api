package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Struct for the API credentials from the config.json file
type JSONConfigData struct {
	Url      string   `json:url`
	Username string   `json:username`
	Password string   `json:password`
	Projects []string `json:projects`
	Teams    []struct {
		Name       string   `json:name`
		TeamLeader string   `json:teamLeader`
		Members    []string `json:members`
	} `json:teams`
}

type jiraDataStruct struct {
	Total  int `json:total`
	Issues []struct {
		Id     string `json:id`
		Self   string `json:self`
		Key    string `json:key`
		Fields struct {
			TimeSpent            int `json:timespent`
			TimeOriginalEstimate int `json:timeoriginalestimate`
			Status               struct {
				Name string `json:name`
			}
			CustomField_11200 struct {
				Value string `json:value`
			} `json:customfield_11200`
			FixVersions []struct {
				Name     string `json:name`
				Id       string `json:id`
				Self     string `json:self`
				Released bool   `json:released`
			} `json:fixVersions`
		} `json:fields`
		ChangeLog struct {
			Histories []struct {
				Items []struct {
					Field      string `json:field`
					FromString string `json:fromString`
					ToString   string `json:toString`
				} `json:items`
			} `json:histories`
		} `json:changelog`
	} `json:issues`
}

var robi_wg sync.WaitGroup

func main() {

	report := flag.String("report", "", "Report to run")
	flag.Parse()

	t := time.Now()
	date := t.Format("01/02/2006")

	year, month := getWeekNumber("2015-03-31T10:27:37.898-0400", "T")
	fmt.Println(year, month)

	// get the config data
	config := &JSONConfigData{}
	J, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("ERROR:Cannot read from config file")
	}
	json.Unmarshal([]byte(J), &config)

	// Connect to Redis
	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {
		fmt.Println("ERROR: Cannot connect to Redis")
	}

	if *report == "defectRatio" {
		for _, team := range config.Teams {
			redisConn.Do("SADD", "teams", team.Name)
			var teamTotal float64 = 0
			var teamPop int = 0
			for _, developer := range team.Members {
				redisConn.Do("SADD", "developers", developer)
				redisConn.Do("SADD", "team:"+team.Name+":developers", developer)
				ratio := getDeveloperDefectRatio(config, developer)
				redisConn.Do("HSET", "stats:"+developer+":defectRatio", date, ratio)
				teamTotal = teamTotal + ratio
				teamPop++
			}
			teamAvg := teamTotal / float64(teamPop)
			redisConn.Do("HSET", "stats:"+team.Name+":defectRatio", date, teamAvg)
		}
	}
}

func getWeekNumber(dateString string, delimiter string) (int, int) {
	s := strings.Split(dateString, delimiter)
	t, _ := time.Parse("2006-01-02", s[0])
	year, week := t.ISOWeek()
	return week, year
}

func cURLEndpoint(config *JSONConfigData, endpoint string) string {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		fmt.Println("ERROR:Cannot connect to EP")
	}
	req.SetBasicAuth(config.Username, config.Password)
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		fmt.Println("ERROR:Cannot authenticate to Jira API")
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println("ERROR:Cannot read API response")
	}
	res.Body.Close()
	return string(body)
}

func getDeveloperDefectRatio(config *JSONConfigData, developer string) float64 {

	endpoint := config.Url
	endpoint += "search?jql=assignee="
	endpoint += developer
	endpoint += "&maxResults=2000"
	endpoint += "&expand=changelog"
	endpoint += "&orderby=created"
	jiraApiResponse := cURLEndpoint(config, endpoint)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	var delivered int = 0
	var rejected int = 0

	for _, issue := range jiraStoryData.Issues {
		for _, history := range issue.ChangeLog.Histories {
			for _, item := range history.Items {
				if item.Field == "status" && item.FromString == "Accepted" && item.ToString == "Rejected" {
					rejected++
				}

				if item.Field == "status" && item.ToString == "Accepted" {
					delivered++
				}
			}
		}
	}
	result := float64(rejected) / float64(delivered)
	return result
}

func getDeveloperWorklog(config *JSONConfigData, developer string) {

	endpoint := config.Url
	endpoint += "search?jql=assignee="
	endpoint += developer
	endpoint += "&maxResults=2000"
	endpoint += "&expand=changelog"
	endpoint += "&orderby=created"
	jiraApiResponse := cURLEndpoint(config, endpoint)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		fmt.Println(issue)
	}
}
