package mockdropbox

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"math/rand"
	gopath "path"
	"strings"
	"sync"
	"time"

	dropboxclient "github.com/koofr/go-dropboxclient"
	pathutils "github.com/koofr/go-pathutils"
)

var randomIdRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type Item struct {
	Metadata *dropboxclient.Metadata
	ParentId string
	Children []*Item
	Data     []byte
	Hash     string
	ChangeID int64
}

type Cursor struct {
	Path         string
	Recursive    bool
	LastChangeID int64
}

type UploadSession struct {
	Id     string
	Buffer *bytes.Buffer
}

func normalizePath(path string) string {
	if strings.HasSuffix(path, "/") {
		return path[:len(path)-1]
	}
	return path
}

func pathToLower(path string) string {
	return strings.ToLower(path)
}

func randomString() string {
	str := make([]rune, 22)
	for i := range str {
		str[i] = randomIdRunes[rand.Intn(len(randomIdRunes))]
	}
	return string(str)
}

func generateId() string {
	return "id:" + randomString()
}

type Store struct {
	itemsByIds      map[string]*Item
	itemsByPaths    map[string]*Item
	deletedItems    []*Item
	uploadSessions  map[string]*UploadSession
	currentChangeID int64
	spaceUsed       int64
	spaceAllocated  int64

	mutex sync.RWMutex
}

func NewStore() *Store {
	s := &Store{
		itemsByIds:      map[string]*Item{},
		itemsByPaths:    map[string]*Item{},
		deletedItems:    []*Item{},
		uploadSessions:  map[string]*UploadSession{},
		currentChangeID: 0,
		spaceUsed:       0,
		spaceAllocated:  2 * 1024 * 1024 * 1024,
	}

	rootItem := &Item{
		Metadata: &dropboxclient.Metadata{
			Id:        "",
			PathLower: "",
			Name:      "",
		},
		ParentId: "",
		Children: []*Item{},
		ChangeID: 0,
	}

	s.itemsByIds[""] = rootItem
	s.itemsByPaths[""] = rootItem

	return s
}

func (s *Store) TimeNow() time.Time {
	return time.Now().UTC()
}

func (s *Store) nextChangeID() int64 {
	s.currentChangeID++
	return s.currentChangeID
}

func (s *Store) GetCurrentChangeID() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.currentChangeID
}

func (s *Store) GetSpaceUsage() (spaceUsed int64, spaceAllocated int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.spaceUsed, s.spaceAllocated
}

func (s *Store) deleteMetadata(md *dropboxclient.Metadata) {
	s.deletedItems = append(s.deletedItems, &Item{
		Metadata: &dropboxclient.Metadata{
			Tag:       dropboxclient.MetadataDeleted,
			Name:      md.Name,
			PathLower: md.PathLower,
		},
		ChangeID: s.nextChangeID(),
	})
}

func (s *Store) CreateFolder(parentItem *Item, path string) (item *Item, ok bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	pathLower := pathToLower(path)

	for _, child := range parentItem.Children {
		if child.Metadata.PathLower == pathLower {
			return nil, false
		}
	}

	md := &dropboxclient.Metadata{
		Tag:       "folder",
		Id:        generateId(),
		Name:      gopath.Base(path),
		PathLower: pathLower,
	}

	childItem := &Item{
		Metadata: md,
		ParentId: parentItem.Metadata.Id,
		Children: []*Item{},
		ChangeID: s.nextChangeID(),
	}
	parentItem.Children = append(parentItem.Children, childItem)
	s.itemsByIds[md.Id] = childItem
	s.itemsByPaths[md.PathLower] = childItem

	return childItem, true
}

func (s *Store) CreateFile(session *UploadSession, parentItem *Item, path string, autorename bool, clientModifiedOpt *time.Time, mode string, modeUpdate string) (newItem *Item, ok bool, isConflict bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data := session.Buffer.Bytes()
	md5Bytes := md5.Sum(data)
	hash := hex.EncodeToString(md5Bytes[:])
	parentPath := gopath.Dir(path)
	pathLower := pathToLower(path)
	name := gopath.Base(path)
	modified := s.TimeNow()
	rev := randomString()
	size := int64(len(data))

	clientModified := modified
	if clientModifiedOpt != nil {
		clientModified = *clientModifiedOpt
	}

	var existingItem *Item
	existingNames := map[string]*Item{}
	for _, child := range parentItem.Children {
		existingNames[pathToLower(child.Metadata.Name)] = child
		if child.Metadata.PathLower == pathLower {
			existingItem = child
		}
	}

	if existingItem != nil && (mode == dropboxclient.WriteModeOverwrite || existingItem.Hash == hash) {
		if existingItem.Metadata.Tag == dropboxclient.MetadataFolder {
			return nil, false, true
		}
		newItem = existingItem
		newItem.Metadata.ClientModified = clientModified
		newItem.Metadata.ServerModified = modified
		newItem.Metadata.Rev = rev
		newItem.Metadata.Size = size
		newItem.Data = data
		newItem.Hash = hash
		newItem.ChangeID = s.nextChangeID()
	} else {
		if existingItem != nil {
			if autorename {
				nameExists := func(name string) bool {
					_, ok := existingNames[pathToLower(name)]
					return ok
				}
				var err error
				name, err = pathutils.UnusedFilename(nameExists, name, 1000)
				if err != nil {
					return nil, false, false
				}
			} else {
				return nil, false, true
			}
		}

		md := &dropboxclient.Metadata{
			Tag:            "file",
			Id:             generateId(),
			Name:           name,
			PathLower:      pathToLower(gopath.Join(parentPath, name)),
			ClientModified: clientModified,
			ServerModified: modified,
			Rev:            rev,
			Size:           size,
		}
		newItem = &Item{
			Metadata: md,
			ParentId: parentItem.Metadata.Id,
			Children: []*Item{},
			Data:     data,
			Hash:     hash,
			ChangeID: s.nextChangeID(),
		}
		parentItem.Children = append(parentItem.Children, newItem)
		s.itemsByIds[md.Id] = newItem
		s.itemsByPaths[md.PathLower] = newItem
	}

	return newItem, true, false
}

func (s *Store) Delete(item *Item) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if parentItem, ok := s.itemsByIds[item.ParentId]; ok {
		newChildren := []*Item{}
		for _, child := range parentItem.Children {
			if child != item {
				newChildren = append(newChildren, child)
			}
		}
		parentItem.Children = newChildren
	}
	var deleteFromItems func(item *Item)
	deleteFromItems = func(item *Item) {
		delete(s.itemsByIds, item.Metadata.Id)
		delete(s.itemsByPaths, item.Metadata.PathLower)
		s.deleteMetadata(item.Metadata)

		for _, child := range item.Children {
			deleteFromItems(child)
		}
	}
	deleteFromItems(item)
}

func (s *Store) copyMetadata(md *dropboxclient.Metadata, newPath string) *dropboxclient.Metadata {
	var newModified time.Time
	if md.Tag == dropboxclient.MetadataFile {
		newModified = s.TimeNow()
	}
	return &dropboxclient.Metadata{
		Tag:            md.Tag,
		Id:             generateId(),
		Name:           gopath.Base(newPath),
		PathLower:      pathToLower(newPath),
		ClientModified: md.ClientModified,
		ServerModified: newModified,
	}
}

func (s *Store) Copy(item *Item, newParentItem *Item, newPath string) *Item {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var cp func(item *Item, newParentItem *Item, newPath string) *Item
	var copyChildren func(item *Item, newItem *Item)

	cp = func(item *Item, newParentItem *Item, newPath string) *Item {
		newItem := &Item{
			Metadata: s.copyMetadata(item.Metadata, newPath),
			ParentId: newParentItem.Metadata.Id,
			Children: []*Item{},
			ChangeID: s.nextChangeID(),
		}
		s.itemsByIds[newItem.Metadata.Id] = newItem
		s.itemsByPaths[newItem.Metadata.PathLower] = newItem
		newParentItem.Children = append(newParentItem.Children, newItem)
		copyChildren(item, newItem)
		return newItem
	}

	copyChildren = func(parent *Item, newParentItem *Item) {
		for _, child := range parent.Children {
			cp(child, newParentItem, gopath.Join(newParentItem.Metadata.PathLower, child.Metadata.Name))
		}
	}

	return cp(item, newParentItem, newPath)
}

func (s *Store) Move(item *Item, newParentItem *Item, newPath string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var mv func(item *Item, newParentItem *Item, newPath string)
	var moveChildren func(item *Item)

	mv = func(item *Item, newParentItem *Item, newPath string) {
		if oldParent, ok := s.itemsByIds[item.ParentId]; ok {
			newChildren := []*Item{}
			for _, child := range oldParent.Children {
				if child != item {
					newChildren = append(newChildren, child)
				}
			}
			oldParent.Children = newChildren
		}
		s.deleteMetadata(item.Metadata)

		delete(s.itemsByPaths, item.Metadata.PathLower)
		item.Metadata.Name = gopath.Base(newPath)
		item.Metadata.PathLower = pathToLower(newPath)
		if item.Metadata.Tag == dropboxclient.MetadataFile {
			item.Metadata.ServerModified = s.TimeNow()
		}
		item.ParentId = newParentItem.Metadata.Id
		item.ChangeID = s.nextChangeID()
		s.itemsByPaths[item.Metadata.PathLower] = item

		newParentItem.Children = append(newParentItem.Children, item)

		moveChildren(item)
	}

	moveChildren = func(parent *Item) {
		for _, child := range parent.Children {
			mv(child, parent, gopath.Join(parent.Metadata.PathLower, child.Metadata.Name))
		}
	}

	mv(item, newParentItem, newPath)
}

func (s *Store) GetItemByPath(path string) (item *Item, ok bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	item, ok = s.itemsByPaths[pathToLower(normalizePath(path))]
	return item, ok
}

func (s *Store) GetDeletedItems() []*Item {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.deletedItems
}

func (s *Store) AddSession(session *UploadSession) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.uploadSessions[session.Id] = session
}

func (s *Store) GetSession(id string) (session *UploadSession, ok bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	session, ok = s.uploadSessions[id]
	return session, ok
}

func (s *Store) DeleteSession(session *UploadSession) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	delete(s.uploadSessions, session.Id)
}
