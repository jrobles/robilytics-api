package main

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
			IssueType            struct {
				Name string `json:string`
			}
			Status struct {
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
