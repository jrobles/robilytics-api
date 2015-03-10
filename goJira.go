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
	Url        string   `json:url`
	Username   string   `json:username`
	Password   string   `json:password`
	Projects   []string `json:projects`
	Developers []string `json:developers`
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

func main() {

	// get the config data
	config := &JSONConfigData{}
	J, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("ERROR:Cannot read from config file")
	}
	json.Unmarshal([]byte(J), &config)

	for _, developer := range config.Developers {
		fmt.Println(developer, developerDefectRatioReport(config, developer))
	}
	/*
		for _, project := range config.Projects {
			sprintEfficiencyReport(config, project)
		}
	*/
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

func teamIterationReport() {}

func developerDefectRatioReport(config *JSONConfigData, developer string) float64 {

	//redisConn, err := redis.Dial("tcp", ":6379")
	//if err != nil {
	//	fmt.Println("ERROR: Cannot connect to Redis")
	//}

	jiraApiResponse1 := getStoriesForDeveloper(config, developer)
	jiraStoryData1 := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse1), &jiraStoryData1)

	var total int = 0
	var rejects int = 0

	for _, issue := range jiraStoryData1.Issues {
		for _, history := range issue.ChangeLog.Histories {
			for _, item := range history.Items {
				if item.Field == "status" && item.FromString == "Accepted" && item.ToString == "Rejected" {
					rejects++
				}

				if item.Field == "status" && item.ToString == "Accepted" {
					total++
				}
			}
		}
	}
	result := float64(rejects) / float64(total)
	return result
	//fmt.Printf("%f\n", result)
}

func sprintEfficiencyReport(config *JSONConfigData, project string) {

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
}

/*
func writeToCsv() {
	csvfile, err := os.Create("output.csv")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer csvfile.Close()

	records := [][]string{{"item1", "value1"}, {"item2", "value2"}, {"item3", "value3"}}

	writer := csv.NewWriter(csvfile)
	for _, record := range records {
		err := writer.Write(record)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
	}
	writer.Flush()
}
*/
