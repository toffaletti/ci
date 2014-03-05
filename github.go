package main

type Owner struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Repository struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	CloneUrl string `json:"clone_url"`
	Owner    Owner  `json:"owner"`
}

type Branch struct {
	Label string     `json:"label"`
	Ref   string     `json:"ref"`
	Sha   string     `json:"sha"`
	Repo  Repository `json:"repo"`
}

type PullRequest struct {
	Url   string `json:"url"`
	Id    int    `json:"id"`
	State string `json:"state"`
	Base  Branch `json:"base"`
	Head  Branch `json:"head"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
}
