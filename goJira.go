package main

import (
	"fmt"
	"io/ioutil"
	"encoding/json"
	"net/http"
	"github.com/garyburd/redigo/redis"
	"strconv"
)

// Struct for the API credentials from the config.json file
type JSONConfigData struct {
	Url string `json:url`
	Username string `json:username`
	Password string `json:password`
}

// Struct used to store the Jira API response
type jiraResponse struct {
	Total	int	`json:total`
	Issues []jiraIssue `json:issues`
}

// Another struct for Jira API response
type jiraIssue struct {
	Id string `json:id`
	Self string `json:self`
	Key string `json:key`
	Fields struct {
		Summary string `json:summary`
		Reporter struct {
			Name string `json:name`
			DisplayName string `json:displayName`
		} `json:reporter`
		Status struct {
			Name string `json:name`
		} `json:status`
		Assignee struct {
			Name string `json:name`
			DisplayName string `json:displayName`
		} `json:assignee`
	} `json:fields`
}

// Struct used to store the developer info
type developerInfo struct {
	Self string `json:self`
	Key string `json:string`
	DisplayName string `json:string`
	Developer string
}

// Function used to save the developer info in the info:{DEVELOPER} Redis HASH
func (this *developerInfo) save(c redis.Conn) {
	redis.Strings(c.Do("HMSET","info:" + this.Developer,"URL",this.Self,"Key",this.Key,"DisplayName",this.DisplayName))
}

func main() {

	// get the config data
	config := &JSONConfigData{}
	J, err := ioutil.ReadFile("config.json")
	if err != nil {fmt.Println("ERROR:Cannot read from config file")}
	json.Unmarshal([]byte(J),&config)

	// Connect to Redis
	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {fmt.Println("ERROR: Cannot connect to Redis")}

	// Populate tasks for each developer
	developers, err := redis.Strings(redisConn.Do("SMEMBERS", "developers"))
	if err != nil {fmt.Println("ERROR: Cannot get data from developers SET")}

	updateRedisData(developers,redisConn,config)

	// Print totals for each developer
	getTotalsForDevelopers(redisConn,developers)
}

func getTotalsForDevelopers(redisConn redis.Conn,developers []string) {

	// Iterate through the developers
	for _,developer := range developers {
		fmt.Println(" ")

		// Get the total number of tasks for the current developer
		devTaskTotal,err := redis.Int(redisConn.Do("SCARD","tasks:" + developer))
		if err != nil {fmt.Println("ERROR: Cannot get tasks for:",developer)}

		// Get the developer display name
		devName,err:= redis.Strings(redisConn.Do("HMGET","info:" + developer,"DisplayName"))
		if err != nil {fmt.Println("ERROR: Cannot get DisplayName for:",developer)}

		// Print the table header
		fmt.Println(devName[0],":",devTaskTotal,"total tasks")
		fmt.Println("--------------------------------------------------------------------------------")

		// List of statuses
		devStatuses,err := Map(redisConn.Do("HGETALL","taskStatuses:" + developer))
		if err != nil {fmt.Println("ERROR: Cannot get tasks for:",developer)}
		for k,devStatus := range devStatuses {
			if k == "Rejected" {
				fmt.Println("-","\033[01;31m",k,":",devStatus,"\033[00m")
			} else {
				fmt.Println("-",k,":",devStatus)
			}
		}

		// prin the progress bar for the current developer
		progressBar(devStatuses,devTaskTotal)
	}
}

// Function used to get all the stories for a specific developer
func getStoriesForDeveloper(config *JSONConfigData,developerUsername string) (string) {
	endpoint := config.Url
	endpoint += "search?jql=assignee="
	endpoint += developerUsername
	endpoint += "&maxResults=2000"
	data :=cURLEndpoint(config,endpoint)
	return data
}

// Function used to clean up our Redis db
func deleteFromRedis (redisConn redis.Conn,objectName string) {
	redis.Strings(redisConn.Do("DEL",objectName))
}

// Function used to get the user information for a specific developer
func getDeveloperInfo (config *JSONConfigData,developerUsername string) (string){
	endpoint := config.Url
	endpoint += "user?username="
	endpoint += developerUsername
	data := cURLEndpoint(config,endpoint)
	return data
}

// Function used to cURL the Jira EP.
func cURLEndpoint(config *JSONConfigData,endpoint string) (string) {
	req, err := http.NewRequest("GET",endpoint, nil)
	if err != nil {fmt.Println("ERROR:Cannot connect to EP")}
	req.SetBasicAuth(config.Username,config.Password)
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {fmt.Println("ERROR:Cannot authenticate to Jira API")}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {fmt.Println("ERROR:Cannot read API response")}
	res.Body.Close()
	return string(body)
}

// Function used to neatly format the data in a Redis HASH
func Map(do_result interface{}, err error) (map[string] string, error){
	result := make(map[string] string, 0)
	a, err := redis.Values(do_result, err)
	if err != nil {
		return result, err
	}
	for len(a) > 0 {
		var key string
		var value string
		a, err = redis.Scan(a, &key, &value)
		if err != nil {
			return result, err
		}
		result[key] = value
	}
	return result, nil
}

// Function used to generate the developer progress bar
func progressBar (devStatuses map[string]string,devTaskTotal int) {
	fmt.Println("--------------------------------------------------------------------------------")
	progressBar := ""
	oof := 0
	for k,storyCount := range devStatuses {
		if (k == "Accepted") {
			i, err := strconv.Atoi(storyCount)
			if err != nil {fmt.Println("ERROR: Cannot convert story count to INT")}
			oof = oof + i
		}
	}
	percentage := oof * 100 / devTaskTotal
	bars := 80 * percentage / 100
	for i := 0; i < bars; i++ {
		progressBar += "\033[01;32m"
		progressBar += "|"
		progressBar += "\033[00m"
	}
	fmt.Println(progressBar)
	fmt.Println("--------------------------------------------------------------------------------")
}

// Function used to store all the applicable user data from the Jira API
func updateRedisData(developers []string,redisConn redis.Conn,config *JSONConfigData) {
	for _,developer := range developers {

		// Get developer info from Jira API
		jiraUserDataResponse := getDeveloperInfo(config,developer)
		jiraUserData := &developerInfo{Developer: developer}
		json.Unmarshal([]byte(jiraUserDataResponse),&jiraUserData)

		// Add developer info to info<developer> HASH
		deleteFromRedis(redisConn,"info:"+developer)

		jiraUserData.save(redisConn)

		// Delete the taskStatuses<developer> HASH
		deleteFromRedis(redisConn,"taskStatuses:" + developer)

		// Get the stories for the current developer
		jiraStoryDataResponse := getStoriesForDeveloper(config,developer)
		jiraStoryData := &jiraResponse{}
		json.Unmarshal([]byte(jiraStoryDataResponse),&jiraStoryData)

		// Delete out the tasks:<developer>SET before populating it
		deleteFromRedis(redisConn,"tasks:"+developer)

		// Iterate through the stories for the current developer
		for _,issue := range jiraStoryData.Issues {

			// Add the developers tasks to the tasks:<developer> SET
			redis.Strings(redisConn.Do("SADD","tasks:" + issue.Fields.Assignee.Name,issue.Id))

			// Add the task details to the task:<task_id> SET
			redis.Strings(redisConn.Do("HSET","task:" + issue.Id,"ID",issue.Id))
			redis.Strings(redisConn.Do("HSET","task:" + issue.Id,"Title",issue.Fields.Summary))
			redis.Strings(redisConn.Do("HSET","task:" + issue.Id,"Status",issue.Fields.Status.Name))

			// Set the status HASH for each developer
			redis.Strings(redisConn.Do("HINCRBY","taskStatuses:" + issue.Fields.Assignee.Name,issue.Fields.Status.Name,1))
		}
	}
}
