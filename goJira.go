package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"log"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Struct for the API credentials from the config.json file
type JSONConfigData struct {
	Url           string   `json:url`
	Username      string   `json:username`
	Password      string   `json:password`
	EmailAddress  string   `json:emailAddress`
	EmailPassword string   `json:emailPassword`
	Projects      []string `json:projects`
	Teams         []struct {
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
			TimeSpent            int    `json:timespent`
			TimeOriginalEstimate int    `json:timeoriginalestimate`
			Updated              string `json:updated`
			Status               struct {
				Name string `json:name`
			}
			CustomField_11200 struct {
				Value string `json:value`
			} `json:customfield_11200`
			CustomField_10700 struct {
				Value string `json:value`
			} `json:customfield_10700`
			FixVersions []struct {
				Name     string `json:name`
				Id       string `json:id`
				Self     string `json:self`
				Released bool   `json:released`
			} `json:fixVersions`
		} `json:fields`
		ChangeLog struct {
			Histories []struct {
				Created string `json:created`
				Items   []struct {
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
		errorToLog("ERROR:Cannot read from config file")
	}
	json.Unmarshal([]byte(J), &config)

	// Connect to Redis
	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {
		errorToLog("ERROR: Cannot connect to Redis")
	}

	switch *report {
	case "velocity":
		numDevelopers, err := redis.Int(redisConn.Do("SCARD", "data:developers"))
		if err != nil {
			errorToLog("ERROR: Cannot retreive number of developers from data:developers SET")
		}
		robi_wg.Add(numDevelopers)
		for _, team := range config.Teams {
			for _, developer := range team.Members {
				go getDeveloperVelocity(config, developer)
			}
		}
		robi_wg.Wait()

	case "meetings":
		numDevelopers, err := redis.Int(redisConn.Do("SCARD", "data:developers"))
		if err != nil {
			errorToLog("ERROR: Cannot retreive number of developers from data:developers SET")
		}
		robi_wg.Add(numDevelopers)
		for _, team := range config.Teams {
			for _, developer := range team.Members {
				go getWorklogData(config, developer)
			}
		}
		robi_wg.Wait()

	case "defectRatio":
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
	case "noEstimateWork":
		numDevelopers, err := redis.Int(redisConn.Do("SCARD", "data:developers"))
		if err != nil {
			errorToLog("ERROR: Cannot retreive number of developers from data:developers SET")
		}
		robi_wg.Add(numDevelopers)
		for _, team := range config.Teams {
			for _, developer := range team.Members {
				go getActiveStoriesWithNoEstimate(config, developer, "Doing")
			}
		}
		robi_wg.Wait()
	}
}

func getWeekNumber(dateString string, delimiter string) (int, int) {
	s := strings.Split(dateString, delimiter)
	t, err := time.Parse("2006-01-02", s[0])
	if err != nil {
		errorToLog("ERROR: Cannot parse date")
	}
	week, year := t.ISOWeek()
	return week, year
}

func cURLEndpoint(config *JSONConfigData, endpoint string) string {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		errorToLog("ERROR:Cannot connect to EP")
	}
	req.SetBasicAuth(config.Username, config.Password)
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		errorToLog("ERROR: Cannot authenticate to JIRA API")
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		errorToLog("ERROR:Cannot read API response")
	}
	res.Body.Close()
	return string(body)
}

func getDeveloperVelocity(config *JSONConfigData, developer string) {

	var y string = "doop"
	var w string = "yyy"

	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {
		errorToLog("ERROR: Cannot connect to Redis")
	}

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
		check, err := redis.Int(redisConn.Do("SISMEMBER", "data:velocityLogs:developer:"+developer, issue.Id))
		if err != nil {
			errorToLog("ERROR: Cannot check if issue is in data:velocityLogs:" + developer + "hash")
		}
		if check == 0 {
			for _, history := range issue.ChangeLog.Histories {
				year, week := getWeekNumber(history.Created, "T")
				y = strconv.Itoa(year)
				w = strconv.Itoa(week)
				for _, item := range history.Items {
					if item.Field == "status" && item.ToString == "Finished" && issue.Fields.TimeSpent > 0 {
						redisConn.Do("HINCRBY", "data:velocity:developer:"+developer, w+":"+y+":TOTAL", issue.Fields.TimeSpent)
						redisConn.Do("HINCRBY", "data:velocity:developer:"+developer, w+":"+y+":ENTRIES", 1)
					}
				}
			}
			total, err := redis.Int(redisConn.Do("HGET", "data:velocity:developer:"+developer, w+":"+y+":TOTAL"))
			if err != nil {
				errorToLog("ERROR: Cannot get total velocity for " + developer)
			}
			entries, err := redis.Int(redisConn.Do("HGET", "data:velocity:developer:"+developer, w+":"+y+":ENTRIES"))
			if err != nil {
				errorToLog("ERROR: Cannot total velocity entries for " + developer)
			}
			if total > 0 && entries > 0 {
				velocity := (total / entries) / 60
				redisConn.Do("HSET", "stats:velocity:developer:"+developer, w+":"+y, velocity)
				redisConn.Do("SADD", "data:velocityLogs:developer:"+developer, issue.Id)
			} else {
				// Add to exception log / email
			}
		}
	}
	defer robi_wg.Done()
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
		errorToLog("ERROR: Cannot connect to Redis")
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
			check, err := redis.Int(redisConn.Do("SISMEMBER", "data:workLogs:developer:"+developer, worklog.Id))
			if err != nil {
				errorToLog("ERROR: Cannot check developer worklogs for " + developer)
			}
			if check == 0 {
				year, week := getWeekNumber(worklog.Created, "T")
				y := strconv.Itoa(year)
				w := strconv.Itoa(week)
				redisConn.Do("HINCRBY", "stats:meetings:developer:"+developer, w+":"+y, worklog.TimeSpentSeconds/60)
				redisConn.Do("SADD", "data:workLogs:developer:"+developer, worklog.Id)
			}
		}
	}
	defer robi_wg.Done()
}

func encodeRFC2047(String string) string {
	// use mail's rfc2047 to encode any string
	addr := mail.Address{String, ""}
	return strings.Trim(addr.String(), " <>")
}

func sendEmail(config *JSONConfigData, recipient string, body string, subject string) {

	smtpServer := "smtp.gmail.com"
	auth := smtp.PlainAuth(
		"",
		config.EmailAddress,
		config.EmailPassword,
		smtpServer,
	)

	header := make(map[string]string)
	header["Return-Path"] = "no-reply@robilytics.net"
	header["From"] = "no-reply@robilytics.net"
	header["To"] = recipient
	header["Subject"] = encodeRFC2047(subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	err := smtp.SendMail(
		smtpServer+":587",
		auth,
		"no-reply@robilytics.net",
		[]string{recipient},
		[]byte(message),
	)
	if err != nil {
		fmt.Println(err)
	}
}

func getActiveStoriesWithNoEstimate(config *JSONConfigData, developer string, status string) {
	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status="
	ep += status
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		if issue.Fields.TimeOriginalEstimate == 0 || issue.Fields.CustomField_10700.Value != "Yes" {
			body := "Story: " + issue.Key
			body += "\r\n"
			body += "Assignee: " + developer
			sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Active stories with no estimate")
		}
	}

	defer robi_wg.Done()
}

func errorToLog(logData string) {
	f, err := os.OpenFile("/var/log/robyLytics.error.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("ERROR: Cannot write to log file")
	}
	defer f.Close()
	log.SetOutput(f)
	log.Println(logData)
}
