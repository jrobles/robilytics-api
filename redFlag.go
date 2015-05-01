package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func getStoriesForProject(config *JSONConfigData, projectName string) string {
	ep := config.Url
	ep += "search?jql=project="
	ep += projectName
	ep += "&maxResults=2000"
	ep += "&expand=changelog"
	ep += "&orderby=created"
	data := cURLEndpoint(config, ep)
	return data
}

func getActiveStoryEdits(config *JSONConfigData, project string) {
	today := time.Now()
	var body string = ""
	var count int = 0

	fieldsToLookFor := map[string]bool{
		"Can You Estimate?":   true,
		"timeestimate":        true,
		"description":         true,
		"Acceptance Criteria": true,
	}

	jiraApiResponse := getStoriesForProject(config, project)
	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		for _, history := range issue.ChangeLog.Histories {
			for _, item := range history.Items {
				if fieldsToLookFor[item.Field] {
					created := strings.Split(history.Created, "T")
					t, err := time.Parse("2006-01-02", created[0])
					if err != nil {
						errorToLog("Could not parse time string", err)
					}
					delta := today.Sub(t)
					ageOfChange := delta.Hours() / 24
					if ageOfChange < 1 {
						body += item.Field + ": " + history.Created + ": https://kreatetechnology.atlassian.net/browse/" + issue.Key
						body += "\r\n"
						count++
					}
				}
			}
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Story Edit: "+project)
	}
	defer project_wg.Done()
}

func getActiveStoriesWithNoEstimate(config *JSONConfigData, developer string) {

	typesToInclude := map[string]bool{
		"Chore": true,
		"Bug":   true,
		"Story": true,
	}

	var body string = ""
	var count int = 0

	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status=Doing"
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		estimate := strconv.Itoa(issue.Fields.TimeOriginalEstimate)
		if estimate == "" || issue.Fields.CustomField_10700.Value != "Yes" {
			if typesToInclude[issue.Fields.IssueType.Name] {
				body += "Story: https://kreatetechnology.atlassian.net/browse/" + issue.Key
				body += "\r\n"
				count++
			}
		}
		if count > 0 {
			sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Active stories with no estimate: "+developer)
		}
	}

	defer developer_wg.Done()
}

func getSubtaskHrsLogged(config *JSONConfigData, subtasks []string) int {
	var totalHrs int = 0
	for _, subtask := range subtasks {
		fmt.Println(subtask)
	}
	return totalHrs
}

func getStoriesWithNoLoggedHrs(config *JSONConfigData, developer string) {

	var body string = ""
	var count int = 0

	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status%20in%20(Accepted%2CDelivered%2CAccepted%2CRejected)"
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		if issue.Fields.TimeSpent <= 0 && len(issue.Fields.Subtasks) == 0 {
			body += "Story: https://kreatetechnology.atlassian.net/browse/" + issue.Key
			body += "\r\n"
			count++
		} else if issue.Fields.TimeSpent <= 0 && len(issue.Fields.Subtasks) > 0 {
			// @TODO NEED TO HANDLE THIS CASE WHERE THE HRS ARE LOGGED IN THE SUBTASKS
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Delivered stories with no time logged: "+developer)
	}
	defer developer_wg.Done()
}

func activeStoriesWithNoFixVersion(config *JSONConfigData, developer string) {

	var body string = ""
	var count int = 0

	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status%20in%20(Accepted%2CDelivered%2CAccepted%2CRejected%2CStarted%2CDoing)"
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		for _, fixVersion := range issue.Fields.FixVersions {
			if fixVersion.Name == "" {
				body += "Story: https://kreatetechnology.atlassian.net/browse/" + issue.Key
				body += "\r\n"
				count++
			}
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Active stories with no fixVersion: "+developer)
	}
	defer developer_wg.Done()
}
