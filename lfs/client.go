package lfs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	
	"github.com/github/git-lfs/git"
)

const (
	mediaType = "application/vnd.git-lfs+json; charset=utf-8"
)

var (
	lfsMediaTypeRE  = regexp.MustCompile(`\Aapplication/vnd\.git\-lfs\+json(;|\z)`)
	jsonMediaTypeRE = regexp.MustCompile(`\Aapplication/json(;|\z)`)
	hiddenHeaders   = map[string]bool{
		"Authorization": true,
	}

	defaultErrors = map[int]string{
		400: "Client error: %s",
		401: "Authorization error: %s\nCheck that you have proper access to the repository",
		403: "Authorization error: %s\nCheck that you have proper access to the repository",
		404: "Repository or object not found: %s\nCheck that it exists and that you have proper access to it",
		500: "Server error: %s",
	}
)

type objectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *objectError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

type objectResource struct {
	Oid     string                   `json:"oid,omitempty"`
	Size    int64                    `json:"size"`
	Actions map[string]*linkRelation `json:"actions,omitempty"`
	Links   map[string]*linkRelation `json:"_links,omitempty"`
	Error   *objectError             `json:"error,omitempty"`
}

func (o *objectResource) Rel(name string) (*linkRelation, bool) {
	var rel *linkRelation
	var ok bool

	if o.Actions != nil {
		rel, ok = o.Actions[name]
	} else {
		rel, ok = o.Links[name]
	}

	return rel, ok
}

type linkRelation struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header,omitempty"`
}

type ClientError struct {
	Message          string `json:"message"`
	DocumentationUrl string `json:"documentation_url,omitempty"`
	RequestId        string `json:"request_id,omitempty"`
}

func (e *ClientError) Error() string {
	msg := e.Message
	if len(e.DocumentationUrl) > 0 {
		msg += "\nDocs: " + e.DocumentationUrl
	}
	if len(e.RequestId) > 0 {
		msg += "\nRequest ID: " + e.RequestId
	}
	return msg
}

type byteCloser struct {
	*bytes.Reader
}

func (b *byteCloser) Close() error {
	return nil
}

func UploadObject(oid string, cb CopyCallback) error {
	path, err := LocalMediaPath(oid)
	if err != nil {
		return Error(err)
	}

	fmt.Fprintf(os.Stderr, "Going to upload %s ...\n", oid)

	file, err := os.Open(path)
	if err != nil {
		return Error(err)
	}
	defer file.Close()

	var bucket =  git.Config.Find("filter.lfs.s3.bucket")
	var region = git.Config.Find("filter.lfs.s3.region")
	var accessKey = git.Config.Find("filter.lfs.s3.accesskey")
	var secretKey = git.Config.Find("filter.lfs.s3.secretkey")
	var blobKey = oid

	client := s3.New(aws.NewConfig().WithRegion(region).WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")))

	uploader := s3manager.NewUploader(&s3manager.UploadOptions{S3: client})
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: &bucket,
		Key:    &blobKey,
		Body:   file,
	})

	if err != nil {
		return Errorf(err, "Error uploading %s.", blobKey)
	}

	fmt.Fprintf(os.Stderr, "Blob url %s\n", result.Location)

	return err
}



func defaultError(res *http.Response) error {
	var msgFmt string

	if f, ok := defaultErrors[res.StatusCode]; ok {
		msgFmt = f
	} else if res.StatusCode < 500 {
		msgFmt = defaultErrors[400] + fmt.Sprintf(" from HTTP %d", res.StatusCode)
	} else {
		msgFmt = defaultErrors[500] + fmt.Sprintf(" from HTTP %d", res.StatusCode)
	}

	return Error(fmt.Errorf(msgFmt, res.Request.URL))
}

func setRequestAuthFromUrl(req *http.Request, u *url.URL) bool {
	if u.User != nil {
		if pass, ok := u.User.Password(); ok {
			fmt.Fprintln(os.Stderr, "warning: current Git remote contains credentials")
			setRequestAuth(req, u.User.Username(), pass)
			return true
		}
	}

	return false
}

func setRequestAuth(req *http.Request, user, pass string) {
	if len(user) == 0 && len(pass) == 0 {
		return
	}

	token := fmt.Sprintf("%s:%s", user, pass)
	auth := "Basic " + base64.URLEncoding.EncodeToString([]byte(token))
	req.Header.Set("Authorization", auth)
}

func setErrorResponseContext(err error, res *http.Response) {
	ErrorSetContext(err, "Status", res.Status)
	setErrorHeaderContext(err, "Request", res.Header)
	setErrorRequestContext(err, res.Request)
}

func setErrorRequestContext(err error, req *http.Request) {
	ErrorSetContext(err, "Endpoint", Config.Endpoint().Url)
	ErrorSetContext(err, "URL", fmt.Sprintf("%s %s", req.Method, req.URL.String()))
	setErrorHeaderContext(err, "Response", req.Header)
}

func setErrorHeaderContext(err error, prefix string, head http.Header) {
	for key, _ := range head {
		contextKey := fmt.Sprintf("%s:%s", prefix, key)
		if _, skip := hiddenHeaders[key]; skip {
			ErrorSetContext(err, contextKey, "--")
		} else {
			ErrorSetContext(err, contextKey, head.Get(key))
		}
	}
}
