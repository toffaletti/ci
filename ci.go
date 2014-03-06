package main

import (
	"code.google.com/p/goauth2/oauth"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// TODO: might need one of these per build-env in the future
var ghClient *github.Client
var userFlag = flag.String("u", "", "github user name for me")
var authFlag = flag.String("a", "", "oauth token")

func handlePullRequest(pre *PullRequestEvent) {
	switch pre.Action {
	case "opened":
		fallthrough
	case "synchronize":
		env := NewBuildEnv(*userFlag, pre)
		if pre.Action == "synchronize" {
			env.CleanComments()
		}
		env.Clone()
		env.Check()
		env.Report()
		env.Clean()
	case "closed":
	case "reopened":
	}
}

func handleEvent(event string, body json.RawMessage) {
	switch event {
	case "pull_request":
		pre := &PullRequestEvent{}
		err := json.Unmarshal(body, pre)
		if err != nil {
			glog.Errorf("json error for pr: %v", err)
		}
		handlePullRequest(pre)
	default:
		glog.V(1).Infof("event: %v body: %v", event, string(body))
	}
}

func main() {
	f := flag.Lookup("logtostderr")
	f.DefValue = "true"
	flag.Set("logtostderr", "true")
	port := flag.Int("p", 1980, "listening port")
	register := flag.String("register", "", "register hook for repo")
	base := flag.String("base", "", "base github url")
	vhost := flag.String("vhost", "", "vhost for this server")
	flag.Parse()
	if *userFlag == "" || *authFlag == "" {
		flag.PrintDefaults()
		return
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: *authFlag},
	}
	ghClient = github.NewClient(t.Client())
	if *base != "" {
		u, err := url.Parse(*base)
		if err != nil {
			panic(err)
		}
		ghClient.BaseURL = u
	}
	if *register != "" {
		if *vhost == "" {
			panic("must set vhost")
		}
		u, err := url.Parse(*register)
		if err != nil {
			panic(err)
		}
		if u.Host != "github.com" {
			ghClient.BaseURL = &url.URL{
				Scheme: u.Scheme,
				Host:   u.Host,
				Path:   "/api/v3/",
			}
			name := "web"
			active := true
			config := map[string]interface{}{
				"url":          fmt.Sprintf("http://%v:%v/", *vhost, *port),
				"content_type": "json",
			}
			hook := &github.Hook{
				Name:   &name,
				Events: []string{"pull_request"},
				Config: config,
				Active: &active,
			}
			splits := strings.SplitN(u.Path, "/", 3)
			owner := splits[1]
			repo := splits[2]
			_, resp, err := ghClient.Repositories.CreateHook(owner, repo, hook)
			if err != nil {
				glog.V(2).Infof("%#v", resp.Response)
				glog.V(2).Infof("%#v", resp.Response.Request)
				panic(err)
			}
		}
		return
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			glog.Errorf("error reading body %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)

		event := r.Header.Get("X-Github-Event")
		switch event {
		case "":
			glog.Info("got unknown request")
			r.Write(os.Stderr)
			return
		default:
			glog.V(3).Infof("request: %#v", *r)
			raw := json.RawMessage{}
			err = raw.UnmarshalJSON(data)
			if err != nil {
				panic(err)
			}
			go handleEvent(event, raw)
		}
	})
	glog.Infof("listening on port %v", *port)
	http.ListenAndServe(fmt.Sprintf(":%v", *port), nil)
}
