package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/google/go-github/github"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type codeMessage struct {
	File string
	Line int
	Msg  string
	Ok   bool // not an error, used for test passing output
}

type BuildEnv struct {
	*Environment
	pre     *PullRequestEvent
	pr      *PullRequest
	fileSet *token.FileSet
	root    string        // repository checkout root
	reports []codeMessage // problems discovered in the code
	user    string        // github user name of me
}

func NewBuildEnv(user string, pre *PullRequestEvent) *BuildEnv {
	u, _ := url.Parse(pre.PullRequest.Base.Repo.CloneUrl)
	sharedGoPath := filepath.Join("/tmp", "ci", "go")
	dir := filepath.Join("/tmp", "ci", pre.PullRequest.Head.Sha)
	return &BuildEnv{
		Environment: NewEnvironment(sharedGoPath, dir),
		pre:         pre,
		pr:          &pre.PullRequest,
		fileSet:     token.NewFileSet(),
		root:        filepath.Join(dir, "src", u.Host, filepath.Dir(u.Path), pre.PullRequest.Head.Repo.Name),
		user:        user,
	}
}

// Build checks out the code and runs through all the stages of vetting the code
func (e *BuildEnv) Build() {
	// TODO: git diff --name-status pr.Base.Sha pr.Head.Sha
	// to get list of changed files only
	log.Printf("pr: %v", e.pr.Url)
	log.Printf("want to commit to %v from %v", e.pr.Base.Label, e.pr.Head.Label)
	dir := filepath.Dir(e.root)
	// clean up any existing checkout at this path
	os.RemoveAll(e.GoPaths[1])
	err := os.MkdirAll(dir, os.ModePerm)
	log.Printf("cloning to %v", dir)
	//c := e.Command("git", "clone", "--quiet", "-b", e.pr.Head.Ref, "--single-branch", e.pr.Head.Repo.CloneUrl)
	// XXX: had to stop doing --single-branch because go get -u runs 'git checkout master'
	c := e.Command("git", "clone", "--quiet", "-b", e.pr.Head.Ref, e.pr.Head.Repo.CloneUrl)
	c.Dir = dir
	err = c.Run()
	if err != nil {
		log.Printf("error cloning: %v", err)
	}
	dir = filepath.Join(dir, e.pr.Head.Repo.Name)
	msgs, err := e.processDir(e.root)
	if err != nil {
		log.Printf("error: %v", err)
	}
	// make file comment paths relative to repo root
	for i, m := range msgs {
		rel, _ := filepath.Rel(e.root, m.File)
		msgs[i].File = rel
	}
	e.reports = msgs
}

// CleanComments removes any outdated issue comments
func (e *BuildEnv) CleanComments() {
	comments, _, err := ghClient.Issues.ListComments(e.pr.Base.Repo.Owner.Login, e.pr.Base.Repo.Name, e.pre.Number, nil)
	if err != nil {
		log.Printf("error listing comments: %v", err)
	}
	for _, comment := range comments {
		if *comment.User.Login == e.user {
			_, err := ghClient.Issues.DeleteComment(e.pr.Base.Repo.Owner.Login, e.pr.Base.Repo.Name, *comment.ID)
			if err != nil {
				log.Printf("error deleting comments: %v", err)
			}
		}
	}
}

// Report makes comments on pull request
func (e *BuildEnv) Report() {
	if len(e.reports) > 0 {
		log.Printf("reports: %v", e.reports)
		err := codeComment(e.pre, e.reports)
		if err != nil {
			log.Printf("error commenting: %v", err)
		}
	}
	reports := e.reports
	e.reports = nil
	allOk := true
	for _, m := range reports {
		if !m.Ok {
			allOk = false
			break
		}
	}
	if allOk {
		os.RemoveAll(e.GoPaths[1])
		log.Printf("all files ok")
	} else if e.pre.PullRequest.State != "closed" {
		// close the pull request
		closed := "closed"
		pr := &github.PullRequest{
			Title: &e.pr.Title,
			Body:  &e.pr.Body,
			State: &closed,
		}
		_, _, err := ghClient.PullRequests.Edit(e.pr.Base.Repo.Owner.Login, e.pr.Base.Repo.Name, e.pre.Number, pr)
		if err != nil {
			log.Printf("error closing pull request: %v", err)
		}
	}
}

func (e *BuildEnv) processFile(path string) (*codeMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseFile(e.fileSet, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	ast.SortImports(e.fileSet, file)
	var buf bytes.Buffer
	pc := &printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 8,
	}
	err = pc.Fprint(&buf, e.fileSet, file)
	if err != nil {
		return nil, err
	}
	res := buf.Bytes()
	if !bytes.Equal(src, res) {
		// formatting changed, save file and run diff on it to find the first changed block
		// use the line number from the diff to comment on the pull request
		ioutil.WriteFile(path+".fmt", res, os.ModePerm)
		lineNumber, err := e.findFirstChange(path, path+".fmt")
		// TODO: show correct fmt?
		if err != nil {
			return nil, fmt.Errorf("%v needs gofmt", filepath.Base(path))
		}
		return &codeMessage{
			File: filepath.Base(path),
			Line: lineNumber + 1,
			Msg:  "gofmt",
		}, nil
	}
	return nil, nil
}

func (e *Environment) findFirstChange(f1 string, f2 string) (lineNumber int, err error) {
	args := []string{
		"--unchanged-line-format=\"\"",
		"--new-line-format=\":%dn: %L\"",
		f1,
		f2,
	}
	c := e.Command("diff", args...)
	out, err := c.CombinedOutput()
	if err != nil {
		// TODO: exit status 1 is expected, but other errors arent
		//if len(out) > 0 {
		//	log.Printf("error running diff: %s", string(out))
		//}
		//return nil, fmt.Errorf("error running diff %v", err)
	}
	splits := strings.SplitN(string(out), "\n", 2)
	if len(splits) != 2 {
		err = errors.New("error parsing diff splits != 2")
		return
	}
	splits = strings.SplitN(splits[1], ":", 3)
	if len(splits) != 3 {
		err = errors.New("error parsing diff splits != 3")
		return
	}
	lineNumber, err = strconv.Atoi(splits[1])
	return
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

func (e *BuildEnv) build(dirs map[string][]string) (msgs []codeMessage, buildPass bool) {
	for dir, _ := range dirs {
		c := e.Command("go", "get", "-d", "-u", "-t")
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			log.Printf("error running go get: %s", string(out))
		}
	}
	// after running go get -u we will be on branch master
	// need to checkout the right branch
	c := e.Command("git", "checkout", e.pr.Head.Ref)
	c.Dir = filepath.Join(filepath.Dir(e.root), e.pr.Head.Repo.Name)
	out, err := c.CombinedOutput()
	if err != nil {
		log.Printf("error checking out branch: %v", string(out))
	}

	// go build packages
	buildPass = true
	for dir, _ := range dirs {
		c := e.Command("go", "build")
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			log.Printf("error running go build: %v", err)
			buildPass = false
			msgs = append(msgs, parseBuildOut(dir, string(out))...)
		}
		// TODO: parse output to go build and comment on first line that fails to build
		//msgs = append(msgs, codeMessage{
		//	Msg: string(out),
		//	Ok:  err == nil,
		//})
	}
	return
}

func (e *BuildEnv) processDir(path string) (msgs []codeMessage, err error) {
	dirs, err := makeTree(path)
	if err != nil {
		return
	}
	// run go vet on each package directory
	for dir, _ := range dirs {
		c := e.Command("go", "vet")
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			//return nil, fmt.Errorf("error running go vet %v", err)
		}
		if len(out) > 0 {
			msgs = parseVetOut(dir, bytes.NewReader(out))
		}
	}

	// process files with gofmt-like logic
	for dir, files := range dirs {
		for _, file := range files {
			msg, err := e.processFile(filepath.Join(dir, file))
			if err != nil {
				return nil, err
			}
			if msg != nil {
				msgs = append(msgs, *msg)
			}
		}
	}

	// if code passed vetting, try to build and run tests
	if len(msgs) == 0 {
		m, buildPass := e.build(dirs)
		msgs = append(msgs, m...)

		// go test packages
		if buildPass {
			for dir, _ := range dirs {
				c := e.Command("go", "test", "-cover")
				c.Dir = dir
				out, err := c.CombinedOutput()
				if err != nil {
					log.Printf("error running go test: %v", err)
				}
				msgs = append(msgs, codeMessage{
					Msg: string(out),
					Ok:  err == nil,
				})
			}
		}
	}
	return msgs, nil
}

func codeComment(pre *PullRequestEvent, msgs []codeMessage) error {
	// TODO: git blame -p -L 241,241 ci.go
	// to get the correct commit
	var comments []string
	for _, m := range msgs {
		if m.File != "" && m.Line > 0 {
			comments = append(comments, fmt.Sprintf("%v:%v: %v", m.File, m.Line, strings.TrimSpace(m.Msg)))
		} else if m.File != "" {
			comments = append(comments, fmt.Sprintf("%v: %v", m.File, strings.TrimSpace(m.Msg)))
		} else if m.Msg != "" {
			comments = append(comments, strings.TrimSpace(m.Msg))
		}
	}
	if len(comments) > 0 {
		// put inside markdown code block to prevent it from being interpreted as markdwon
		commentBody := fmt.Sprintf("```\n%s\n```", strings.Join(comments, "\n"))
		err := issueComment(pre, commentBody)
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

func parseBuildOut(dir string, out string) (msgs []codeMessage) {
	// TODO: don't care about line numbers until github improves pull request comments
	//splits := strings.SplitN(string(out), "\n", 3)
	//if len(splits) == 3 {
	//	//pkgPath := splits[0][2:]
	//	splits := strings.SplitN(splits[1], ": ", 2)
	//	if len(splits) == 2 {
	//		errMsg := splits[1]
	//		splits = strings.SplitN(splits[0], ":", 2)
	//		if len(splits) == 2 {
	//			lineNumber, _ := strconv.Atoi(splits[1])
	//			msgs = append(msgs, codeMessage{
	//				Line: lineNumber + 1,
	//				File: filepath.Join(dir, splits[0]),
	//				Msg:  errMsg,
	//			})
	//		}
	//	}
	//}

	msgs = append(msgs, codeMessage{
		Msg: strings.Replace(out, dir, "", -1),
	})
	return
}

func parseVetOut(dir string, r io.Reader) (msgs []codeMessage) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
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
			File: filepath.Join(dir, filename),
			Line: lineNumber + 1,
			Msg:  msg,
		})
	}
	return
}
