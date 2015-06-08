package dropboxclient

import (
	"fmt"
	"github.com/koofr/go-httpclient"
	"github.com/koofr/go-ioutils"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type Dropbox struct {
	ApiHTTPClient     *httpclient.HTTPClient
	ContentHTTPClient *httpclient.HTTPClient
}

func NewDropbox(authToken string) (dropbox *Dropbox) {
	apiBaseUrl, _ := url.Parse("https://api.dropbox.com")
	contentBaseUrl, _ := url.Parse("https://api-content.dropbox.com")

	apiHttpClient := httpclient.New()
	apiHttpClient.BaseURL = apiBaseUrl
	apiHttpClient.Headers.Set("Authorization", "Bearer "+authToken)

	contentHttpClient := httpclient.New()
	contentHttpClient.BaseURL = contentBaseUrl
	contentHttpClient.Headers.Set("Authorization", "Bearer "+authToken)

	return &Dropbox{
		ApiHTTPClient:     apiHttpClient,
		ContentHTTPClient: contentHttpClient,
	}
}

func (c *Dropbox) Info(path string) (info *DropboxFile, err error) {
	res, err := c.ContentHTTPClient.Request(&httpclient.RequestData{
		Method:         "HEAD",
		Path:           "/1/files/auto/" + path,
		ExpectedStatus: []int{http.StatusOK},
		RespConsume:    true,
	})

	if err != nil {
		return
	}

	info = DropboxFileFromHeaders(path, res.Header)

	return
}

func (c *Dropbox) Get(path string, span *ioutils.FileSpan) (file *DropboxFile, err error) {
	req := httpclient.RequestData{
		Method:         "GET",
		Path:           "/1/files/auto/" + path,
		ExpectedStatus: []int{http.StatusOK, http.StatusPartialContent},
	}

	if span != nil {
		req.Headers = make(http.Header)
		req.Headers.Set("Range", fmt.Sprintf("bytes=%d-%d", span.Start, span.End))
	}

	res, err := c.ContentHTTPClient.Request(&req)

	if err != nil {
		return
	}

	file = DropboxFileFromHeaders(path, res.Header)
	file.Reader = res.Body

	return
}

func (c *Dropbox) ChunkedUpload(uploadId string, offset int64, reader io.Reader) (res *ChunkedUploadResult, err error) {
	params := make(url.Values)
	if uploadId != "" {
		params.Set("upload_id", uploadId)
	}
	if offset != 0 {
		params.Set("offset", strconv.FormatInt(offset, 10))
	}

	_, err = c.ContentHTTPClient.Request(&httpclient.RequestData{
		Method:         "PUT",
		Path:           "/1/chunked_upload",
		Params:         params,
		ReqReader:      reader,
		ExpectedStatus: []int{http.StatusOK},
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &res,
	})

	return
}

func (c *Dropbox) CommitChunkedUpload(path string, uploadId string, overwrite bool) (res *CommitChunkedUploadResult, err error) {
	params := make(url.Values)
	params.Set("upload_id", uploadId)
	params.Set("overwrite", fmt.Sprintf("%t", overwrite))

	_, err = c.ContentHTTPClient.Request(&httpclient.RequestData{
		Method:         "POST",
		Path:           "/1/commit_chunked_upload/auto/" + path,
		Params:         params,
		ExpectedStatus: []int{http.StatusOK},
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &res,
	})

	return
}
