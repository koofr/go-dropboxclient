package mockdropbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	gopath "path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/koofr/go-dropboxclient"
	httputils "github.com/koofr/go-httputils"
	ioutils "github.com/koofr/go-ioutils"
	"github.com/koofr/go-pathutils"
)

type MockDropbox struct {
	handler http.Handler

	stores      map[string]*Store
	storesMutex sync.Mutex
}

func New() *MockDropbox {
	d := &MockDropbox{
		stores: map[string]*Store{},
	}

	r := mux.NewRouter()
	r.Methods("POST").Path("/2/users/get_space_usage").HandlerFunc(d.UsersGetSpaceUsage)
	r.Methods("POST").Path("/2/files/create_folder").HandlerFunc(d.FilesCreateFolder)
	r.Methods("POST").Path("/2/files/get_metadata").HandlerFunc(d.FilesGetMetadata)
	r.Methods("POST").Path("/2/files/list_folder").HandlerFunc(d.FilesListFolder)
	r.Methods("POST").Path("/2/files/list_folder/continue").HandlerFunc(d.FilesListFolderContinue)
	r.Methods("POST").Path("/2/files/delete").HandlerFunc(d.FilesDelete)
	r.Methods("POST").Path("/2/files/copy").HandlerFunc(d.FilesCopy)
	r.Methods("POST").Path("/2/files/move").HandlerFunc(d.FilesMove)
	r.Methods("POST").Path("/2/files/upload_session/start").HandlerFunc(d.FilesUploadSessionStart)
	r.Methods("POST").Path("/2/files/upload_session/append").HandlerFunc(d.FilesUploadSessionAppend)
	r.Methods("POST").Path("/2/files/upload_session/finish").HandlerFunc(d.FilesUploadSessionFinish)
	r.Methods("POST").Path("/2/files/download").HandlerFunc(d.FilesDownload)

	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not found", http.StatusNotFound)
	})

	d.handler = r

	return d
}

func (d *MockDropbox) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.handler.ServeHTTP(w, r)
}

func (d *MockDropbox) AccessToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return auth[7:]
	}
	return auth
}

func (d *MockDropbox) Store(r *http.Request) *Store {
	d.storesMutex.Lock()
	defer d.storesMutex.Unlock()

	token := d.AccessToken(r)

	store, ok := d.stores[token]
	if !ok {
		store = NewStore()
		d.stores[token] = store
	}

	return store
}

func (d *MockDropbox) validPath(w http.ResponseWriter, path string) bool {
	if !pathutils.IsPathValid(path) {
		http.Error(w, "Invalid path", http.StatusInternalServerError)
		return false
	}
	return true
}

func (d *MockDropbox) buildCursor(path string, recursive bool, lastChangeID int64) string {
	cursor := &Cursor{
		Path:         path,
		Recursive:    recursive,
		LastChangeID: lastChangeID,
	}
	data, _ := json.Marshal(cursor)
	return base64.StdEncoding.EncodeToString(data)
}

func (d *MockDropbox) invalidCursor(w http.ResponseWriter) {
	d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
		ErrorSummary: "path/not_found/..",
		Err: dropboxclient.DropboxErrorDetails{
			Tag: "path",
			Path: &dropboxclient.LookupError{
				Tag: "not_found",
			},
		},
	})
}

func (d *MockDropbox) parseCursor(w http.ResponseWriter, cursor string) (c *Cursor, ok bool) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		http.Error(w, "Error in call to API function \"files/list_folder/continue\": Invalid \"cursor\" parameter: '"+cursor+"'", http.StatusBadRequest)
		return nil, false
	}
	c = &Cursor{}
	err = json.Unmarshal(data, &c)
	if err != nil {
		http.Error(w, "Error in call to API function \"files/list_folder/continue\": Invalid \"cursor\" parameter: '"+cursor+"'", http.StatusBadRequest)
		return nil, false
	}
	return c, true
}

func (d *MockDropbox) arg(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("arg read error: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	err = json.Unmarshal(data, v)
	if err != nil {
		log.Printf("arg json unmarshal error: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func (d *MockDropbox) headerArg(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	data := []byte(r.Header.Get("Dropbox-API-Arg"))
	err := json.Unmarshal(data, v)
	if err != nil {
		log.Printf("header arg json unmarshal error: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func (d *MockDropbox) pathNotFound(w http.ResponseWriter) {
	d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
		ErrorSummary: "path/not_found/..",
		Err: dropboxclient.DropboxErrorDetails{
			Tag: "path",
			Path: &dropboxclient.LookupError{
				Tag: "not_found",
			},
		},
	})
}

func (d *MockDropbox) pathLookupNotFound(w http.ResponseWriter) {
	d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
		ErrorSummary: "path_lookup/not_found/",
		Err: dropboxclient.DropboxErrorDetails{
			Tag: "path_lookup",
			PathLookup: &dropboxclient.LookupError{
				Tag: "not_found",
			},
		},
	})
}

func (d *MockDropbox) res(w http.ResponseWriter, statusCode int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("res json marshal error: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(data)
}

func (d *MockDropbox) headerRes(w http.ResponseWriter, v interface{}) bool {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("res json marshal error: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	w.Header().Set("Dropbox-Api-Result", string(data))
	return true
}

func (d *MockDropbox) UsersGetSpaceUsage(w http.ResponseWriter, r *http.Request) {
	spaceUsed, spaceAllocated := d.Store(r).GetSpaceUsage()

	d.res(w, http.StatusOK, &dropboxclient.SpaceUsage{
		Used: spaceUsed,
		Allocation: &dropboxclient.SpaceAllocation{
			Tag:       ".individual",
			Allocated: spaceAllocated,
		},
	})
}

func (d *MockDropbox) FilesCreateFolder(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.CreateFolderArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.Path) {
		return
	}
	parentPath := gopath.Dir(arg.Path)
	parentItem, ok := d.Store(r).GetItemByPath(parentPath)
	if !ok {
		d.pathNotFound(w)
		return
	}
	item, ok := d.Store(r).CreateFolder(parentItem, arg.Path)
	if !ok {
		d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
			ErrorSummary: "path/conflict/file/...",
			Err: dropboxclient.DropboxErrorDetails{
				Tag: "path",
				Path: &dropboxclient.LookupError{
					Tag: "conflict",
				},
			},
		})
		return
	}
	mdCopy := *item.Metadata
	mdCopy.Tag = ""
	d.res(w, http.StatusOK, mdCopy)
}

func (d *MockDropbox) FilesGetMetadata(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.GetMetadataArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.Path) {
		return
	}
	item, ok := d.Store(r).GetItemByPath(arg.Path)
	if !ok {
		d.pathNotFound(w)
		return
	}
	if item.Metadata.Id == "" {
		http.Error(w, "Error in call to API function \"files/get_metadata\": request body: path: The root folder is unsupported.", http.StatusBadRequest)
		return
	}
	d.res(w, http.StatusOK, item.Metadata)
}

func (d *MockDropbox) listFolder(w http.ResponseWriter, r *http.Request, cursor *Cursor) {
	nextChangeID := d.Store(r).GetCurrentChangeID()
	item, ok := d.Store(r).GetItemByPath(cursor.Path)
	if !ok {
		d.pathNotFound(w)
		return
	}
	items := []*Item{}
	addEntry := func(item *Item) {
		if item.ChangeID > cursor.LastChangeID {
			items = append(items, item)
		}
	}
	var addEntries func(item *Item)
	addEntries = func(item *Item) {
		for _, child := range item.Children {
			addEntry(child)
			if cursor.Recursive {
				addEntries(child)
			}
		}
	}
	if cursor.Recursive && item.Metadata.Id != "" {
		addEntry(item)
	}
	addEntries(item)
	if cursor.LastChangeID != 0 {
		for _, item := range d.Store(r).GetDeletedItems() {
			addEntry(item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ChangeID < items[j].ChangeID
	})
	entries := make([]*dropboxclient.Metadata, len(items))
	for i, item := range items {
		entries[i] = item.Metadata
	}
	d.res(w, http.StatusOK, &dropboxclient.ListFolderResult{
		Entries: entries,
		HasMore: false,
		Cursor:  d.buildCursor(cursor.Path, cursor.Recursive, nextChangeID),
	})
}

func (d *MockDropbox) FilesListFolder(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.ListFolderArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.Path) {
		return
	}
	cursor := &Cursor{
		Path:         arg.Path,
		Recursive:    arg.Recursive,
		LastChangeID: 0,
	}
	d.listFolder(w, r, cursor)
}

func (d *MockDropbox) FilesListFolderContinue(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.ListFolderContinueArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	cursor, ok := d.parseCursor(w, arg.Cursor)
	if !ok {
		return
	}
	d.listFolder(w, r, cursor)
}

func (d *MockDropbox) FilesDelete(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.DeleteArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.Path) {
		return
	}
	item, ok := d.Store(r).GetItemByPath(arg.Path)
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	d.Store(r).Delete(item)
	d.res(w, http.StatusOK, item.Metadata)
}

func (d *MockDropbox) FilesCopy(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.RelocationArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.FromPath) {
		return
	}
	if !d.validPath(w, arg.ToPath) {
		return
	}
	item, ok := d.Store(r).GetItemByPath(arg.FromPath)
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	newParentItem, ok := d.Store(r).GetItemByPath(gopath.Dir(arg.ToPath))
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	newItem := d.Store(r).Copy(item, newParentItem, arg.ToPath)
	d.res(w, http.StatusOK, newItem.Metadata)
}

func (d *MockDropbox) FilesMove(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.RelocationArg{}
	if !d.arg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.FromPath) {
		return
	}
	if !d.validPath(w, arg.ToPath) {
		return
	}
	item, ok := d.Store(r).GetItemByPath(arg.FromPath)
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	newParentItem, ok := d.Store(r).GetItemByPath(gopath.Dir(arg.ToPath))
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	d.Store(r).Move(item, newParentItem, arg.ToPath)
	d.res(w, http.StatusOK, item.Metadata)
}

func (d *MockDropbox) FilesUploadSessionStart(w http.ResponseWriter, r *http.Request) {
	session := &UploadSession{
		Id:     randomString(),
		Buffer: bytes.NewBuffer(nil),
	}
	_, err := io.Copy(session.Buffer, r.Body)
	if err != nil {
		http.Error(w, "Upload copy error", http.StatusInternalServerError)
		return
	}
	d.Store(r).AddSession(session)
	d.res(w, http.StatusOK, &dropboxclient.UploadSessionStartResult{
		SessionId: session.Id,
	})
}

func (d *MockDropbox) FilesUploadSessionAppend(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.UploadSessionCursor{}
	if !d.headerArg(w, r, &arg) {
		return
	}
	session, ok := d.Store(r).GetSession(arg.SessionId)
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid session id: %s", arg.SessionId), http.StatusInternalServerError)
		return
	}
	session.Buffer.Truncate(int(arg.Offset))
	_, err := io.Copy(session.Buffer, r.Body)
	if err != nil {
		http.Error(w, "Upload copy error", http.StatusInternalServerError)
		return
	}
	d.res(w, http.StatusOK, nil)
}

func (d *MockDropbox) FilesUploadSessionFinish(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.UploadSessionFinishArg{}
	if !d.headerArg(w, r, &arg) {
		return
	}
	session, ok := d.Store(r).GetSession(arg.Cursor.SessionId)
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid session id: %s", arg.Cursor.SessionId), http.StatusInternalServerError)
		return
	}
	if !d.validPath(w, arg.Commit.Path) {
		return
	}
	parentItem, ok := d.Store(r).GetItemByPath(gopath.Dir(arg.Commit.Path))
	if !ok {
		d.pathLookupNotFound(w)
		return
	}
	var clientModifiedOpt *time.Time
	if arg.Commit.ClientModified != nil {
		clientModified, err := time.Parse(dropboxclient.DropboxClientModifiedFormat, *arg.Commit.ClientModified)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid client modified time format: %s", err), http.StatusInternalServerError)
			return
		}
		clientModifiedOpt = &clientModified
	}
	item, ok, isConflict := d.Store(r).CreateFile(session, parentItem, arg.Commit.Path, arg.Commit.Autorename, clientModifiedOpt, arg.Commit.Mode.Tag, arg.Commit.Mode.Update)
	if !ok {
		if isConflict {
			d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
				ErrorSummary: "path/conflict/file/...",
				Err: dropboxclient.DropboxErrorDetails{
					Tag: "path",
					Path: &dropboxclient.LookupError{
						Tag: "conflict",
					},
				},
			})
		} else {
			d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
				ErrorSummary: "other/...",
				Err: dropboxclient.DropboxErrorDetails{
					Tag: "other",
				},
			})
		}
		return
	}
	d.Store(r).DeleteSession(session)
	mdCopy := *item.Metadata
	mdCopy.Tag = ""
	d.res(w, http.StatusOK, mdCopy)
}

func (d *MockDropbox) setupRange(w http.ResponseWriter, r *http.Request, md *dropboxclient.Metadata) (span *ioutils.FileSpan, ok bool) {
	rng := r.Header.Get("Range")
	if rng == "" {
		return nil, true
	}

	spans, _, err := httputils.ParseRange(rng, md.Size)
	if err != nil {
		log.Printf("setupRange error: %s", err)
		http.Error(w, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
		return nil, false
	}

	if len(spans) != 1 {
		log.Printf("setupRange multiple ranges not supported")
		http.Error(w, "Multiple ranges not supported", http.StatusBadRequest)
		return nil, false
	}

	span = &spans[0]

	contentRange := fmt.Sprintf("bytes %d-%d/%d", span.Start, span.End, md.Size)
	w.Header().Set("Content-Range", contentRange)

	return span, true
}

func (d *MockDropbox) FilesDownload(w http.ResponseWriter, r *http.Request) {
	arg := &dropboxclient.DownloadArg{}
	if !d.headerArg(w, r, &arg) {
		return
	}
	if !d.validPath(w, arg.Path) {
		return
	}
	item, ok := d.Store(r).GetItemByPath(arg.Path)
	if !ok {
		d.pathNotFound(w)
		return
	}
	if item.Metadata.Tag != "file" {
		d.res(w, http.StatusConflict, &dropboxclient.DropboxError{
			ErrorSummary: "unsupported_file/...",
			Err: dropboxclient.DropboxErrorDetails{
				Tag: "unsupported_file",
			},
		})
	}
	bytesReader := bytes.NewReader(item.Data)
	var reader io.Reader = bytesReader
	length := int64(len(item.Data))
	span, ok := d.setupRange(w, r, item.Metadata)
	if !ok {
		return
	}
	if span != nil {
		_, err := bytesReader.Seek(span.Start, os.SEEK_SET)
		if err != nil {
			log.Printf("reader seek error: %s", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		length = span.End - span.Start + 1

		reader = io.LimitReader(reader, length)
	}
	if !d.headerRes(w, item.Metadata) {
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
	w.Header().Set("Etag", fmt.Sprintf(`W/"%s"`, item.Metadata.Rev))

	w.WriteHeader(http.StatusOK)

	io.Copy(w, reader)
}
