package main

import (
	"fmt"
	"io/ioutil"
	"encoding/json"
	"net/http"
	"github.com/garyburd/redigo/redis"
	"github.com/tealeg/xlsx"
	"strconv"
	"strings"
	"sort"
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

type jiraDetailResponse struct {
	Id int `json:id`
	Key string `json:key`
	Fields struct {
		TimeTracking struct {
			OriginalEstimateSeconds int `json:originalEstimateSeconds`
			RemainingEstimateSeconds int `json:remainingEstimateSeconds`
			TimeSpentSeconds int `json:timeSpentSeconds`
		} `json:timetracking`
	} `json:fields`
}

// Another struct for Jira API response
type jiraIssue struct {
	Id string `json:id`
	Self string `json:self`
	Key string `json:key`
	Fields struct {
		Summary string `json:summary`
		Customfield_10007 []string `json:customfield_10007`
		IssueType struct {
			Name string `json:name`
		} `json:issueType`
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
		Updated string `json:updated`
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

	writeToXLS(developers,redisConn)

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
		fmt.Println(devName[0],":",devTaskTotal,"total tasks,",getDeveloperHours(developer,redisConn),"hours")

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

// Function used to get additional information for a specific story
func getStoryDetails (config *JSONConfigData,storyID string) (string){
	endpoint := config.Url
	endpoint += "issue/"
	endpoint += storyID
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

// Function used to store all the applicable user data from the Jira API
func updateRedisData(developers []string,redisConn redis.Conn,config *JSONConfigData) {

	var hoursWorked int

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

			if (getSprintStatus(issue.Fields.Customfield_10007) == "ACTIVE") {

				// Add the developers tasks to the tasks:<developer> SET
				redis.Strings(redisConn.Do("SADD","tasks:" + issue.Fields.Assignee.Name,issue.Id))

				// Get the story details
				jiraStoryDetailResponse := getStoryDetails(config,issue.Id)
				jiraStoryDetail := &jiraDetailResponse{}
				json.Unmarshal([]byte(jiraStoryDetailResponse),&jiraStoryDetail)


				redisConn.Do("HSET","task:" + issue.Id,"ID","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"Project","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"Type","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"Key","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"Title","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"Status","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"OriginalEstimate","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"RemainingEstimate","NULL")
				redisConn.Do("HSET","task:" + issue.Id,"TimeSpent","NULL")

				// Add the task details to the task:<task_id> SET
				redisConn.Do("HSET","task:" + issue.Id,"ID",issue.Id)
				redisConn.Do("HSET","task:" + issue.Id,"Type",issue.Fields.IssueType.Name)
				redisConn.Do("HSET","task:" + issue.Id,"Key",issue.Key)
				redisConn.Do("HSET","task:" + issue.Id,"Title",issue.Fields.Summary)
				redisConn.Do("HSET","task:" + issue.Id,"Status",issue.Fields.Status.Name)
				redisConn.Do("HSET","task:" + issue.Id,"OriginalEstimate",jiraStoryDetail.Fields.TimeTracking.OriginalEstimateSeconds /60/60)
				redisConn.Do("HSET","task:" + issue.Id,"RemainingEstimate",jiraStoryDetail.Fields.TimeTracking.RemainingEstimateSeconds /60/60)
				redisConn.Do("HSET","task:" + issue.Id,"TimeSpent",jiraStoryDetail.Fields.TimeTracking.TimeSpentSeconds/60/60)

				// Set the status HASH for each developer
				redisConn.Do("HINCRBY","taskStatuses:" + issue.Fields.Assignee.Name,issue.Fields.Status.Name,1)

				hoursWorked  = hoursWorked + jiraStoryDetail.Fields.TimeTracking.TimeSpentSeconds

			}
		}
	}
}

// Function used to get the total hours worked by a developer
func getDeveloperHours(developer string,redisConn redis.Conn) int {
	var hoursWorked int
	stories,_ := redis.Strings(redisConn.Do("SMEMBERS", "tasks:"+developer))
	for _,story := range stories {
		storyDetails,_ := Map(redisConn.Do("HGETALL","task:" + story))
		for k,storyDetail := range storyDetails {
			if (k == "TimeSpent") {
				seconds,_ := strconv.Atoi(storyDetail)
				//hoursWorked = hoursWorked + seconds / 60 / 60
				hoursWorked = hoursWorked + seconds
			}
		}
	}
	return hoursWorked
}

// Function used to get the status of a sprint
func getSprintStatus (jiraString []string) string {
	var status string
	for _,v := range jiraString {
		s := strings.Split(v,",")
		sprintStatus := strings.Split(s[1],"=")
		status = sprintStatus[1]
	}
		return status
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

func writeToXLS(developers []string,redisConn redis.Conn) {

	var file *xlsx.File
    var sheet *xlsx.Sheet
    var row *xlsx.Row
    var cell *xlsx.Cell
    var err error

	file = xlsx.NewFile()

	for _,developer := range developers {

		sheet = file.AddSheet(developer)

		// Add header
		row = sheet.AddRow()
		cell = row.AddCell()
		cell.Value = "ID"
		cell = row.AddCell()
		cell.Value = "Key"
		cell = row.AddCell()
		cell.Value = "Original Estimate"
		cell = row.AddCell()
		cell.Value = "Project"
		cell = row.AddCell()
		cell.Value = "Remaining Estimate"
		cell = row.AddCell()
		cell.Value = "Status"
		cell = row.AddCell()
		cell.Value = "Time Spent"
		cell = row.AddCell()
		cell.Value = "Title"
		cell = row.AddCell()
		cell.Value = "Type"

		stories,_ := redis.Strings(redisConn.Do("SMEMBERS", "tasks:"+developer))

		for _,story := range stories {
			row = sheet.AddRow()
			storyDetails,_ := Map(redisConn.Do("HGETALL","task:" + story))

			var keys []string
			for k := range storyDetails {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				cell = row.AddCell()
				cell.Value = storyDetails[k]
			}
		}
	}

    err = file.Save("MyXLSXFile.xlsx")
    if err != nil {
        fmt.Printf(err.Error())
    }
}
