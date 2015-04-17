package main

import (
	"encoding/json"
	"strconv"
)

func getActiveStoriesWithNoEstimate(config *JSONConfigData, developer string) {

	var bodyNE string = ""
	var countNE int = 0

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
				bodyNE += "Story: " + issue.Key
				bodyNE += "\r\n"
				countNE++
			}
		}
		if countNE > 0 {
			sendEmail(config, "jose.robles@kreatetechnology.com", bodyNE, "ROBILYTICS: Active stories with no estimate: "+developer)
		}
	}

	defer robi_wg.Done()
}

func getStoriesWithNoLoggedHrs(config *JSONConfigData, developer string) {

	var bodyNWL string = ""
	var countNWL int = 0

	ep := config.Url
	ep += "search?jql=assignee="
	ep += developer
	ep += "%20and%20status%20in%20(Accepted%2CDelivered%2CAccepted%2CRejected)"
	jiraApiResponse := cURLEndpoint(config, ep)

	jiraStoryData := &jiraDataStruct{}
	json.Unmarshal([]byte(jiraApiResponse), &jiraStoryData)

	for _, issue := range jiraStoryData.Issues {
		if issue.Fields.TimeSpent <= 0 {
			bodyNWL += "Story: " + issue.Key
			bodyNWL += "\r\n"
			countNWL++
		}
	}
	if countNWL > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", bodyNWL, "ROBILYTICS: Delivered stories with no time logged: "+developer)
	}
	defer robi_wg.Done()
}
