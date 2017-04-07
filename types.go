package dropboxclient

import (
	"io"
	"time"

	"github.com/koofr/go-httpclient"
)

const SpaceAllocationIndividual = "individual"
const SpaceAllocationTeam = "team"

type SpaceAllocation struct {
	Tag       string `json:".tag"`
	Used      int64  `json:"used"`
	Allocated int64  `json:"allocated"`
}

type SpaceUsage struct {
	Used       int64            `json:"used"`
	Allocation *SpaceAllocation `json:"allocation"`
}

const MetadataFile = "file"
const MetadataFolder = "folder"
const MetadataDeleted = "deleted"

type Metadata struct {
	Tag            string    `json:".tag"`
	Name           string    `json:"name"`
	PathLower      string    `json:"path_lower"`
	ClientModified time.Time `json:"client_modified"`
	ServerModified time.Time `json:"server_modified"`
	Rev            string    `json:"rev"`
	Size           int64     `json:"size"`
	Id             string    `json:"id"`

	ETag          string
	ContentLength int64
}

type GetMetadataArg struct {
	Path             string `json:"path"`
	IncludeMediaInfo bool   `json:"include_media_info"`
}

type ListFolderArg struct {
	Path             string `json:"path"`
	Recursive        bool   `json:"recursive"`
	IncludeMediaInfo bool   `json:"include_media_info"`
	IncludeDeleted   bool   `json:"include_deleted"`
}

type ListFolderResult struct {
	Entries []*Metadata `json:"entries"`
	Cursor  string      `json:"cursor"`
	HasMore bool        `json:"has_more"`
}

type ListFolderContinueArg struct {
	Cursor string `json:"cursor"`
}

type DownloadArg struct {
	Path string `json:"path"`
}

type CreateFolderArg struct {
	Path string `json:"path"`
}

type DeleteArg struct {
	Path string `json:"path"`
}

type RelocationArg struct {
	FromPath string `json:"from_path"`
	ToPath   string `json:"to_path"`
}

type UploadSessionStartResult struct {
	SessionId string `json:"session_id"`
}

type UploadSessionCursor struct {
	SessionId string `json:"session_id"`
	Offset    int64  `json:"offset"`
}

const WriteModeAdd = "add"
const WriteModeOverwrite = "overwrite"
const WriteModeUpdate = "update"

type WriteMode struct {
	Tag    string `json:".tag"`
	Update string `json:"update,omitempty"`
}

type CommitInfo struct {
	Path           string     `json:"path"`
	Mode           *WriteMode `json:"mode"`
	Autorename     bool       `json:"autorename"`
	ClientModified *int64     `json:"client_modified,omitempty"`
	Mute           bool       `json:"mute"`
}

type UploadSessionFinishArg struct {
	Cursor *UploadSessionCursor `json:"cursor"`
	Commit *CommitInfo          `json:"commit"`
}

type DownloadV1 struct {
	ContentLength int64
	ETag          string
	Reader        io.ReadCloser
}

type DropboxErrorDetails struct {
	Tag        string       `json:".tag"`
	Path       *LookupError `json:"path"`
	PathLookup *LookupError `json:"path_lookup"`
}

type LookupError struct {
	Tag string `json:".tag"`
}

type DropboxError struct {
	ErrorSummary    string              `json:"error_summary"`
	Err             DropboxErrorDetails `json:"error"`
	HttpClientError *httpclient.InvalidStatusError
}

func (e *DropboxError) Error() string {
	return e.ErrorSummary
}

func IsDropboxError(err error) (dropboxErr *DropboxError, ok bool) {
	if dbe, ok := err.(*DropboxError); ok {
		return dbe, true
	} else {
		return nil, false
	}
}
