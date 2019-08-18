package dropboxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/koofr/go-httpclient"
	"github.com/koofr/go-ioutils"
)

const (
	DropboxClientModifiedFormat = "2006-01-02T15:04:05Z07"
)

type Dropbox struct {
	ApiHTTPClient     *httpclient.HTTPClient
	ContentHTTPClient *httpclient.HTTPClient
}

func NewDropbox(accessToken string) (dropbox *Dropbox) {
	apiBaseUrl, _ := url.Parse("https://api.dropboxapi.com")
	contentBaseUrl, _ := url.Parse("https://content.dropboxapi.com")

	apiHttpClient := httpclient.New()
	apiHttpClient.BaseURL = apiBaseUrl
	apiHttpClient.Headers.Set("Authorization", "Bearer "+accessToken)

	contentHttpClient := httpclient.New()
	contentHttpClient.BaseURL = contentBaseUrl
	contentHttpClient.Headers.Set("Authorization", "Bearer "+accessToken)

	return &Dropbox{
		ApiHTTPClient:     apiHttpClient,
		ContentHTTPClient: contentHttpClient,
	}
}

func (c *Dropbox) addApiArg(req *httpclient.RequestData, arg interface{}) error {
	argJsonBytes, err := json.Marshal(arg)
	if err != nil {
		return err
	}

	if req.Headers == nil {
		req.Headers = make(http.Header)
	}

	req.Headers.Set("Dropbox-API-Arg", string(argJsonBytes))

	return nil
}

func (c *Dropbox) getApiResult(res *http.Response, result interface{}) error {
	resultJson := res.Header.Get("Dropbox-API-Result")

	err := json.Unmarshal([]byte(resultJson), result)
	if err != nil {
		return err
	}

	return nil
}

func (c *Dropbox) HandleError(err error) error {
	if ise, ok := httpclient.IsInvalidStatusError(err); ok {
		dropboxErr := &DropboxError{}

		if ise.Headers.Get("Content-Type") == "application/json" {
			if jsonErr := json.Unmarshal([]byte(ise.Content), &dropboxErr); jsonErr != nil {
				dropboxErr.ErrorSummary = ise.Content
			}
		} else {
			dropboxErr.ErrorSummary = ise.Content
		}

		if dropboxErr.ErrorSummary == "" {
			dropboxErr.ErrorSummary = ise.Error()
		}

		dropboxErr.HttpClientError = ise

		return dropboxErr
	} else {
		return err
	}
}

func (c *Dropbox) Request(client *httpclient.HTTPClient, req *httpclient.RequestData) (res *http.Response, err error) {
	res, err = client.Request(req)

	if err != nil {
		return res, c.HandleError(err)
	}

	return res, nil
}

func (c *Dropbox) ApiRequest(req *httpclient.RequestData) (res *http.Response, err error) {
	return c.Request(c.ApiHTTPClient, req)
}

func (c *Dropbox) ContentRequest(req *httpclient.RequestData) (res *http.Response, err error) {
	return c.Request(c.ContentHTTPClient, req)
}

func (c *Dropbox) GetSpaceUsage(ctx context.Context) (result *SpaceUsage, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/users/get_space_usage",
		ExpectedStatus: []int{http.StatusOK},
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) GetMetadata(ctx context.Context, arg *GetMetadataArg) (result *Metadata, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/get_metadata",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) ListFolder(ctx context.Context, arg *ListFolderArg) (result *ListFolderResult, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/list_folder",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) ListFolderContinue(ctx context.Context, arg *ListFolderContinueArg) (result *ListFolderResult, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/list_folder/continue",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return nil, err
	}

	return
}

func (c *Dropbox) CreateFolder(ctx context.Context, arg *CreateFolderArg) (result *Metadata, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/create_folder",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) Delete(ctx context.Context, arg *DeleteArg) (result *Metadata, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/delete",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) Copy(ctx context.Context, arg *RelocationArg) (result *Metadata, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/copy",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) Move(ctx context.Context, arg *RelocationArg) (result *Metadata, err error) {
	_, err = c.ApiRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/move",
		ExpectedStatus: []int{http.StatusOK},
		ReqEncoding:    httpclient.EncodingJSON,
		ReqValue:       arg,
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &result,
	})

	if err != nil {
		return
	}

	return
}

func (c *Dropbox) Download(ctx context.Context, arg *DownloadArg, span *ioutils.FileSpan) (reader io.ReadCloser, result *Metadata, err error) {
	req := &httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/download",
		ExpectedStatus: []int{http.StatusOK, http.StatusPartialContent},
	}

	if span != nil {
		req.Headers = make(http.Header)
		req.Headers.Set("Range", fmt.Sprintf("bytes=%d-%d", span.Start, span.End))
	}

	if err = c.addApiArg(req, arg); err != nil {
		return
	}

	res, err := c.ContentRequest(req)

	if err != nil {
		return
	}

	result = &Metadata{}

	if err = c.getApiResult(res, result); err != nil {
		res.Body.Close()
		return nil, nil, err
	}

	result.ETag = res.Header.Get("Etag")

	contentLength, _ := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 0)

	result.ContentLength = contentLength

	return res.Body, result, err
}

func (c *Dropbox) UploadSessionStart(ctx context.Context, reader io.Reader) (res *UploadSessionStartResult, err error) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")

	_, err = c.ContentRequest(&httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/upload_session/start",
		Headers:        headers,
		ReqReader:      reader,
		ExpectedStatus: []int{http.StatusOK},
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &res,
	})

	return
}

func (c *Dropbox) UploadSessionAppend(ctx context.Context, arg *UploadSessionCursor, reader io.Reader) (err error) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")

	req := &httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/upload_session/append",
		Headers:        headers,
		ReqReader:      reader,
		ExpectedStatus: []int{http.StatusOK},
	}

	if err = c.addApiArg(req, arg); err != nil {
		return
	}

	_, err = c.ContentRequest(req)

	return
}

func (c *Dropbox) UploadSessionFinish(ctx context.Context, arg *UploadSessionFinishArg) (res *Metadata, err error) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")

	req := &httpclient.RequestData{
		Context:        ctx,
		Method:         "POST",
		Path:           "/2/files/upload_session/finish",
		Headers:        headers,
		ExpectedStatus: []int{http.StatusOK},
		RespEncoding:   httpclient.EncodingJSON,
		RespValue:      &res,
	}

	if err = c.addApiArg(req, arg); err != nil {
		return
	}

	_, err = c.ContentRequest(req)

	return
}
