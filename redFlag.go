package main

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func getActiveStoriesWithNoEstimate(config *JSONConfigData, developer string) {

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
			body += "Story: " + issue.Key
			body += "\r\n"
			count++
		} else if issue.Fields.TimeSpent <= 0 && len(issue.Fields.Subtasks) > 0 {
			// @TODO NEED TO HANDLE THIS CASE WHERE THE HRS ARE LOGGED IN THE SUBTASKS
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Delivered stories with no time logged: "+developer)
	}
	defer robi_wg.Done()
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
				body += "Story: " + issue.Key
				body += "\r\n"
				count++
			}
		}
	}
	if count > 0 {
		sendEmail(config, "jose.robles@kreatetechnology.com", body, "ROBILYTICS: Active stories with no fixVersion: "+developer)
	}
	defer robi_wg.Done()
}
