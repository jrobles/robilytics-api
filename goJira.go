package main

import (
	"encoding/json"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"net/http"
)

// Struct for the API credentials from the config.json file
type JSONConfigData struct {
	Url      string   `json:url`
	Username string   `json:username`
	Password string   `json:password`
	Projects []string `json:projects`
}

type jiraResponse struct {
	Total  int `json:total`
	Issues []struct {
		Id     string `json:id`
		Self   string `json:self`
		Key    string `json:key`
		Fields struct {
			Assignee struct {
				DisplayName string `json:displayName`
			} `json:assignee`
			IssueType struct {
				Name string `json:name`
			} `json:issuetype`
			Priority struct {
				Name string `json:name`
			} `json:priority`
			Status struct {
				Name string `json:name`
			}
			Project struct {
				Self string `json:self`
				Id   string `json:id`
				Key  string `json:key`
				Name string `json:name`
			} `json:project`
			CustomField_11200 struct {
				Self  string `json:self`
				Value string `json:value`
				Id    string `json:id`
			} `json:customfield_11200`
			FixVersions []struct {
				Name string `json:name`
			} `json:fixVersions`
		} `json:fields`
	} `json:issues`
}

func main() {

	// get the config data
	config := &JSONConfigData{}
	J, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("ERROR:Cannot read from config file")
	}
	json.Unmarshal([]byte(J), &config)

	for _, project := range config.Projects {
		releaseProgressReport(config, project)
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

func releaseProgressReport(config *JSONConfigData, project string) {

	completedStatus := map[string]bool{
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
	jiraStoryData := &jiraResponse{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {

		for _, fixVersion := range issue.Fields.FixVersions {

			if completedStatus[issue.Fields.Status.Name] {
				redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Complete", 1)
			}

			// release info
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Version", fixVersion.Name)
			redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Team", issue.Fields.CustomField_11200.Value)
			redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, issue.Fields.Status.Name, 1)
			redisConn.Do("HINCRBY", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Total", 1)

			// COmpletion percentage calculation
			total, _ := redis.Int(redisConn.Do("HGET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Total"))
			complete, _ := redis.Int(redisConn.Do("HGET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Complete"))

			if total > 0 {
				redisConn.Do("HSET", issue.Fields.CustomField_11200.Value+":"+fixVersion.Name, "Progress", complete*100/total)
			}

		}
	}
}
