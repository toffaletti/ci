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
	"os"
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
	flag.Parse()
	if *userFlag == "" || *authFlag == "" {
		flag.PrintDefaults()
		return
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: *authFlag},
	}
	ghClient = github.NewClient(t.Client())
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
