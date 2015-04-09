package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"net/http"
	"strconv"
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

type jiraWorklogStruct struct {
	Total    int `json:total`
	Worklogs []struct {
		TimeSpentSeconds int    `json:timeSpentSeconds`
		Created          string `json:created`
		Id               string `json:id`
	}
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

	if *report == "meetings" {
		numDevelopers, _ := redis.Int(redisConn.Do("SCARD", "data:developers"))
		robi_wg.Add(numDevelopers)
		for _, team := range config.Teams {
			for _, developer := range team.Members {
				go getWorklogData(config, developer)
			}
		}
		robi_wg.Wait()
	}

	if *report == "defectRatio" {
		t := time.Now()
		date := t.Format("01/02/2006")
		year, week := t.ISOWeek()
		y := strconv.Itoa(year)
		w := strconv.Itoa(week)

		for _, team := range config.Teams {
			redisConn.Do("SADD", "data:teams", team.Name)
			var teamTotal float64 = 0
			var teamPop int = 0
			for _, developer := range team.Members {
				redisConn.Do("SADD", "data:developers", developer)
				redisConn.Do("SADD", "data:team:"+team.Name+":developers:", developer)
				ratio := getDeveloperDefectRatio(config, developer)
				redisConn.Do("HSET", "stats:defectRatio:developer:"+developer, w+":"+y, ratio)
				teamTotal = teamTotal + ratio
				teamPop++
			}
			teamAvg := teamTotal / float64(teamPop)
			redisConn.Do("HSET", "stats:defectRatio:team:"+team.Name, date, teamAvg)
		}
	}
}

func getWeekNumber(dateString string, delimiter string) (int, int) {
	s := strings.Split(dateString, delimiter)
	t, _ := time.Parse("2006-01-02", s[0])
	week, year := t.ISOWeek()
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

func getWorklogData(config *JSONConfigData, developer string) {

	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {
		fmt.Println("ERROR: Cannot connect to Redis")
	}

	ep1 := config.Url
	ep1 += "search?jql=assignee="
	ep1 += developer
	ep1 += "%20and%20issueType=Meeting"
	ep1 += "%20and%20status=Doing"
	jiraApiResponse := cURLEndpoint(config, ep1)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		ep2 := config.Url
		ep2 += "issue/"
		ep2 += issue.Key
		ep2 += "/worklog"
		jiraApiResponse := cURLEndpoint(config, ep2)

		jiraWorklogData := &jiraWorklogStruct{}
		json.Unmarshal([]byte(jiraApiResponse), &jiraWorklogData)

		for _, worklog := range jiraWorklogData.Worklogs {
			check, _ := redis.Int(redisConn.Do("SISMEMBER", "data:workLogs:developer:"+developer, worklog.Id))
			if check == 0 {
				year, week := getWeekNumber(worklog.Created, "T")
				y := strconv.Itoa(year)
				w := strconv.Itoa(week)
				//redisConn.Do("HINCRBY", "stats:"+y+":"+developer+":meetings", week, worklog.TimeSpentSeconds/60)
				redisConn.Do("HINCRBY", "stats:meetings:developer:"+developer, w+":"+y, worklog.TimeSpentSeconds/60)
				redisConn.Do("SADD", "data:workLogs:developer:"+developer, worklog.Id)
			}
		}
	}
	defer robi_wg.Done()
}
