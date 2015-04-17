package main

import (
	"encoding/json"
	"strconv"
)

var body string = ""
var count int = 0

func getActiveStoriesWithNoEstimate(config *JSONConfigData, developer string) {

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
			if issue.Fields.IssueType.Name != "Meeting" {
				body += "Story: " + issue.Key
				body += "\r\n"
				count++
			}
		}
		if count > 0 {
			sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Active stories with no estimate: "+developer)
		}
	}

	defer robi_wg.Done()
}

func getStoriesWithNoLoggedHrs(config *JSONConfigData, developer string) {

	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status%20in%20(Accepted%2CDelivered%2CAccepted%2CRejected)"
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		if issue.Fields.TimeSpent <= 0 {
			body += "Story: " + issue.Key
			body += "\r\n"
			count++
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Delivered stories with no time logged: "+developer)
	}
	defer robi_wg.Done()
}
