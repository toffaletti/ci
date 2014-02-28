package main

import (
	"bufio"
	"bytes"
	"code.google.com/p/goauth2/oauth"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/go-github/github"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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
}

type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
}

// TODO: one fileSet per PR event
var fileSet = token.NewFileSet()
var ghClient *github.Client

func processFile(path string) (*codeMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseFile(fileSet, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	ast.SortImports(fileSet, file)
	var buf bytes.Buffer
	pc := &printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 8,
	}
	err = pc.Fprint(&buf, fileSet, file)
	if err != nil {
		return nil, err
	}
	res := buf.Bytes()
	if !bytes.Equal(src, res) {
		// formatting changed
		// TODO: save file for debugging
		ioutil.WriteFile(path+".fmt", res, os.ModePerm)
		args := []string{
			"--unchanged-line-format=\"\"",
			"--new-line-format=\":%dn: %L\"",
			path,
			path + ".fmt",
		}
		c := exec.Command("diff", args...)
		out, err := c.CombinedOutput()
		if err != nil {
			// TODO: exit status 1 is expected.
			//if len(out) > 0 {
			//	log.Printf("error running diff: %s", string(out))
			//}
			//return nil, fmt.Errorf("error running diff %v", err)
		}
		splits := strings.SplitN(string(out), "\n", 2)
		if len(splits) == 2 {
			splits = strings.SplitN(splits[1], ":", 3)
			if len(splits) == 3 {
				lineNumber, err := strconv.Atoi(splits[1])
				if err != nil {
					return nil, err
				}
				// TODO: show correct fmt?
				return &codeMessage{
					File: filepath.Base(path),
					Line: lineNumber,
					Msg:  "gofmt",
				}, nil
			}
		}
		return nil, fmt.Errorf("%v needs gofmt", filepath.Base(path))
	}
	return nil, nil
}

func makeTree(dir string) (dirs map[string][]string, err error) {
	dirs = make(map[string][]string)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if c := info.Name()[0]; c == '.' || c == '_' {
			return filepath.SkipDir
		}
		if info.Mode()&os.ModeType == 0 {
			// regular file
			if ok, _ := filepath.Match("*.go", info.Name()); ok {
				dir := filepath.Dir(path)
				if files, ok := dirs[dir]; ok {
					dirs[dir] = append(files, info.Name())
				} else {
					dirs[dir] = []string{info.Name()}
				}
			}
		}
		return nil
	})
	return
}

func processDir(path string) (msgs []codeMessage, err error) {
	dirs, err := makeTree(path)
	if err != nil {
		return
	}
	for dir, _ := range dirs {
		os.Chdir(dir)
		c := exec.Command("go", "vet")
		out, err := c.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("error running go vet %v", err)
		}
		if len(out) > 0 {
			msgs = parseVetOut(bytes.NewReader(out))
		}
	}

	// process files with gofmt-like logic
	for dir, files := range dirs {
		for _, file := range files {
			msg, err := processFile(filepath.Join(dir, file))
			if err != nil {
				return nil, err
			}
			if msg != nil {
				msgs = append(msgs, *msg)
			}
		}
	}
	return msgs, nil
}

// TODO: git diff --name-status pr.Base.Sha pr.Head.Sha
// to get list of changed files only

func handlePullRequest(pre *PullRequestEvent) {
	switch pre.Action {
	case "opened":
		fallthrough
	case "synchronize":
		pr := &pre.PullRequest
		log.Printf("pr: %v", pr.Url)
		log.Printf("want to commit to %v from %v", pr.Base.Label, pr.Head.Label)
		u, _ := url.Parse(pr.Base.Repo.CloneUrl)
		dir := filepath.Join("/tmp", "ci", fmt.Sprintf("%v", pr.Head.Sha), "src", u.Host, filepath.Dir(u.Path))
		// clean up any existing checkout at this path
		os.RemoveAll(dir)
		err := os.MkdirAll(dir, os.ModePerm)
		os.Chdir(dir)
		log.Printf("cloning to %v", dir)
		c := exec.Command("git", "clone", "--quiet", "-b", pr.Head.Ref, "--single-branch", pr.Head.Repo.CloneUrl)
		err = c.Run()
		if err != nil {
			log.Printf("error cloning: %v", err)
		}
		msgs, err := processDir(dir)
		if err != nil {
			log.Printf("error: %v", err)
		} else if len(msgs) > 0 {
			log.Printf("comments: %v", msgs)
			err = codeComment(pre, msgs)
			if err != nil {
				log.Printf("error commenting: %v", err)
			}
		} else {
			err = issueComment(pre, "LGTM")
			if err != nil {
				log.Printf("error commenting: %v", err)
			}
			log.Printf("all files ok")
			os.RemoveAll(dir)
		}

	case "closed":
	case "reopened":
	}
}

func codeComment(pre *PullRequestEvent, msgs []codeMessage) error {
	pr := pre.PullRequest
	// TODO: git blame -p -L 241,241 ci.go
	// to get the correct commit
	for _, m := range msgs {
		commentBody := fmt.Sprintf("```\n%s\n```", strings.TrimSpace(m.Msg))
		prc := &github.PullRequestComment{
			Body:     &commentBody,
			CommitID: &pr.Head.Sha,
			Path:     &m.File,
			Position: &m.Line,
		}
		_, _, err := ghClient.PullRequests.CreateComment(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pre.Number, prc)
		if err != nil {
			return err
		}
	}
	return nil
}

func issueComment(pre *PullRequestEvent, commentBody string) error {
	pr := pre.PullRequest
	comment := &github.IssueComment{
		Body: &commentBody,
	}
	_, _, err := ghClient.Issues.CreateComment(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pre.Number, comment)
	return err
}

type codeMessage struct {
	File string
	Line int
	Msg  string
}

func parseVetOut(r io.Reader) (msgs []codeMessage) {
	// TODO: use bufio.Scanner
	rr := bufio.NewReader(r)
	for {
		line, _, err := rr.ReadLine()
		if err != nil {
			break
		}
		splits := strings.SplitN(string(line), ": ", 2)
		if len(splits) != 2 {
			continue
		}
		msg := splits[1]
		splits = strings.Split(splits[0], ":")
		if len(splits) < 2 {
			continue
		}
		filename := splits[0]
		lineNumber, _ := strconv.Atoi(splits[1])
		msgs = append(msgs, codeMessage{
			File: filename,
			Line: lineNumber,
			Msg:  msg,
		})
	}
	return
}

func handleEvent(event string, body json.RawMessage) {
	switch event {
	case "pull_request":
		pre := &PullRequestEvent{}
		err := json.Unmarshal(body, pre)
		if err != nil {
			log.Printf("json error for pr: %v", err)
		}
		handlePullRequest(pre)
	default:
		log.Printf("event: %v body: %v", event, string(body))
	}
}

func main() {
	flag.Parse()
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: flag.Arg(0)},
	}
	ghClient = github.NewClient(t.Client())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading body %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)

		event := r.Header.Get("X-Github-Event")
		switch event {
		case "":
			log.Printf("got unknown request")
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
	log.Print("listening on port 1980")
	http.ListenAndServe(":1980", nil)
}
