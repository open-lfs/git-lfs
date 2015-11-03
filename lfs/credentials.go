package lfs

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

// getCreds gets the credentials for the given request's URL, and sets its
// Authorization header with them using Basic Authentication. This is like
// getCredsForAPI(), but skips checking the LFS url or git remote.
func getCreds(req *http.Request) (Creds, error) {
	if skipCredsCheck(req) {
		return nil, nil
	}

	return fillCredentials(req, req.URL)
}

func skipCredsCheck(req *http.Request) bool {
	if len(req.Header.Get("Authorization")) > 0 {
		return true
	}

	q := req.URL.Query()
	return len(q["token"]) > 0
}

func fillCredentials(req *http.Request, u *url.URL) (Creds, error) {
	path := strings.TrimPrefix(u.Path, "/")
	input := Creds{"protocol": u.Scheme, "host": u.Host, "path": path}
	if u.User != nil && u.User.Username() != "" {
		input["username"] = u.User.Username()
	}

	creds, err := execCreds(input, "fill")

	if creds != nil && err == nil {
		setRequestAuth(req, creds["username"], creds["password"])
	}

	return creds, err
}

func saveCredentials(creds Creds, res *http.Response) {
	if creds == nil {
		return
	}

	switch res.StatusCode {
	case 401, 403:
		execCreds(creds, "reject")
	default:
		if res.StatusCode < 300 {
			execCreds(creds, "approve")
		}
	}
}

type Creds map[string]string

func (c Creds) Buffer() *bytes.Buffer {
	buf := new(bytes.Buffer)

	for k, v := range c {
		buf.Write([]byte(k))
		buf.Write([]byte("="))
		buf.Write([]byte(v))
		buf.Write([]byte("\n"))
	}

	return buf
}

type credentialFunc func(Creds, string) (Creds, error)

func execCredsCommand(input Creds, subCommand string) (Creds, error) {
	output := new(bytes.Buffer)
	cmd := exec.Command("git", "credential", subCommand)
	cmd.Stdin = input.Buffer()
	cmd.Stdout = output
	/*
		There is a reason we don't hook up stderr here:
		Git's credential cache daemon helper does not close its stderr, so if this
		process is the process that fires up the daemon, it will wait forever
		(until the daemon exits, really) trying to read from stderr.

		See https://github.com/github/git-lfs/issues/117 for more details.
	*/

	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}

	if _, ok := err.(*exec.ExitError); ok {
		if !Config.GetenvBool("GIT_TERMINAL_PROMPT", true) {
			return nil, fmt.Errorf("Change the GIT_TERMINAL_PROMPT env var to be prompted to enter your credentials for %s://%s.",
				input["protocol"], input["host"])
		}

		// 'git credential' exits with 128 if the helper doesn't fill the username
		// and password values.
		if subCommand == "fill" && err.Error() == "exit status 128" {
			return input, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("'git credential %s' error: %s\n", subCommand, err.Error())
	}

	creds := make(Creds)
	for _, line := range strings.Split(output.String(), "\n") {
		pieces := strings.SplitN(line, "=", 2)
		if len(pieces) < 2 {
			continue
		}
		creds[pieces[0]] = pieces[1]
	}

	return creds, nil
}

var execCreds credentialFunc = execCredsCommand
