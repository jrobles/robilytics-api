package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"net/http"
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
			Status struct {
				Name string `json:name`
			}
			CustomField_11200 struct {
				Value string `json:value`
			} `json:customfield_11200`
			FixVersions []struct {
				Name string `json:name`
				Id   string `json:id`
				Self string `json:self`
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

	if *report == "progressReport" {
		robi_wg.Add(len(config.Projects))
		for _, project := range config.Projects {
			go progressReport(config, project)
		}
		robi_wg.Wait()
	}

}

func getStoriesForProject(config *JSONConfigData, projectName string) string {
	endpoint := config.Url
	endpoint += "search?jql=project="
	endpoint += projectName
	endpoint += "&maxResults=2000"
	data := cURLEndpoint(config, endpoint)
	return data
}

func getStoriesForDeveloper(config *JSONConfigData, developerName string) string {
	endpoint := config.Url
	endpoint += "search?jql=assignee="
	endpoint += developerName
	endpoint += "&maxResults=2000"
	endpoint += "&expand=changelog"
	data := cURLEndpoint(config, endpoint)
	return data
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

	jiraApiResponse1 := getStoriesForDeveloper(config, developer)
	jiraStoryData1 := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse1), &jiraStoryData1)

	var delivered int = 0
	var rejected int = 0

	for _, issue := range jiraStoryData1.Issues {
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

func progressReport(config *JSONConfigData, project string) {
	completedStatusHaystack := map[string]bool{
		"Finished":  true,
		"Accepted":  true,
		"Delivered": true,
	}

	// Connect to Redis
	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {
		fmt.Println("ERROR: Cannot connect to Redis")
	}

	jiraApiResponse := getStoriesForProject(config, project)
	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {

		for _, fixVersion := range issue.Fields.FixVersions {

			if completedStatusHaystack[issue.Fields.Status.Name] {
				redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Complete", 1)
			}

			// release info
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Version", fixVersion.Name)
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Id", fixVersion.Id)
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Url", fixVersion.Self)
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Team", issue.Fields.CustomField_11200.Value)
			redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, issue.Fields.Status.Name, 1)
			redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Total", 1)

			// Completion percentage calculation
			total, _ := redis.Int(redisConn.Do("HGET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Total"))
			complete, _ := redis.Int(redisConn.Do("HGET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Complete"))

			if total > 0 {
				redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Progress", complete*100/total)
			}
		}
	}
	defer robi_wg.Done()
}
