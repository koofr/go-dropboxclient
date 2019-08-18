package dropboxclient_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	. "github.com/koofr/go-dropboxclient"
	"github.com/koofr/go-dropboxclient/mockdropbox"
	"github.com/koofr/go-ioutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type listResultPair struct {
	tag  string
	path string
}

func listResultPairs(result *ListFolderResult) []listResultPair {
	pairs := []listResultPair{}
	for _, md := range result.Entries {
		pairs = append(pairs, listResultPair{md.Tag, md.PathLower})
	}
	return pairs
}

var _ = Describe("Dropbox", func() {
	var client *Dropbox

	accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")
	useMock := os.Getenv("DROPBOX_USE_MOCK") == "true"

	if accessToken == "" {
		fmt.Println("DROPBOX_ACCESS_TOKEN env variable missing")
		return
	}

	var mockServer *httptest.Server

	BeforeEach(func() {
		rand.Seed(time.Now().UnixNano())

		client = NewDropbox(accessToken)

		if useMock {
			mockServer = httptest.NewServer(mockdropbox.New())
			tsURL, _ := url.Parse(mockServer.URL)
			client.ApiHTTPClient.BaseURL = tsURL
			client.ContentHTTPClient.BaseURL = tsURL
		}
	})

	AfterEach(func() {
		if useMock {
			mockServer.Close()
		}
	})

	var randomName = func() string {
		return fmt.Sprintf("%d", rand.Int())
	}

	var createFolder = func() *Metadata {
		name := randomName()

		md, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + name})
		Expect(err).NotTo(HaveOccurred())
		Expect(md.Name).To(Equal(name))

		return md
	}

	Describe("GetSpaceUsage", func() {
		It("should get space usage", func() {
			usage, err := client.GetSpaceUsage(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(usage.Allocation.Allocated).To(BeNumerically(">", 0))
			Expect(usage.Used).To(BeNumerically(">=", 0))
		})
	})

	Describe("GetMetadata", func() {
		It("should get metadata for path", func() {
			folder := createFolder()

			md, err := client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))
		})
	})

	Describe("ListFolder", func() {
		It("should list root", func() {
			createFolder()

			result, err := client.ListFolder(context.Background(), &ListFolderArg{Path: ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) > 0).To(BeTrue())
		})

		It("should list folder", func() {
			folder := createFolder()

			result, err := client.ListFolder(context.Background(), &ListFolderArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) == 0).To(BeTrue())
		})
	})

	Describe("ListFolderContinue", func() {
		It("should continue folder listing", func() {
			dir1, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + randomName()})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + dir1.Name + "/" + randomName()})
			Expect(err).NotTo(HaveOccurred())

			result, err := client.ListFolder(context.Background(), &ListFolderArg{Path: "/" + dir1.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) > 0).To(BeTrue())

			result, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) >= 0).To(BeTrue())

			_, err = client.Delete(context.Background(), &DeleteArg{Path: "/" + dir1.Name})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).To(HaveOccurred())

			dropboxErr, ok := IsDropboxError(err)
			Expect(ok).To(BeTrue())
			Expect(dropboxErr.Err.Tag).To(Equal("path"))
			Expect(dropboxErr.Err.Path.Tag).To(Equal("not_found"))

			_, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: "invalid"})
			Expect(err).To(HaveOccurred())

			dropboxErr, ok = IsDropboxError(err)
			Expect(ok).To(BeTrue())
			Expect(strings.Contains(dropboxErr.ErrorSummary, `Invalid "cursor"`)).To(BeTrue())
		})

		It("should get deleted items", func() {
			dir1, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + randomName()})
			Expect(err).NotTo(HaveOccurred())

			dir2, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + dir1.Name + "/" + randomName()})
			Expect(err).NotTo(HaveOccurred())

			result, err := client.ListFolder(context.Background(), &ListFolderArg{Path: "/" + dir1.Name, Recursive: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResultPairs(result)).To(ConsistOf(
				listResultPair{MetadataFolder, dir1.PathLower},
				listResultPair{MetadataFolder, dir2.PathLower},
			))
			Expect(result.HasMore).To(BeFalse())

			dir3, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + dir1.Name + "/" + dir2.Name + "/" + randomName()})
			Expect(err).NotTo(HaveOccurred())

			result, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResultPairs(result)).To(ConsistOf(
				listResultPair{MetadataFolder, dir3.PathLower},
			))
			Expect(result.HasMore).To(BeFalse())

			oldDir3 := dir3
			dir3, err = client.Move(context.Background(), &RelocationArg{FromPath: dir3.PathLower, ToPath: path.Join(path.Dir(dir3.PathLower), randomName())})
			Expect(err).NotTo(HaveOccurred())

			result, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResultPairs(result)).To(ConsistOf(
				listResultPair{MetadataDeleted, oldDir3.PathLower},
				listResultPair{MetadataFolder, dir3.PathLower},
			))
			Expect(result.HasMore).To(BeFalse())

			_, err = client.Delete(context.Background(), &DeleteArg{Path: dir3.PathLower})
			Expect(err).NotTo(HaveOccurred())

			result, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResultPairs(result)).To(ConsistOf(
				listResultPair{MetadataDeleted, dir3.PathLower},
			))
			Expect(result.HasMore).To(BeFalse())

			_, err = client.Delete(context.Background(), &DeleteArg{Path: dir2.PathLower})
			Expect(err).NotTo(HaveOccurred())

			result, err = client.ListFolderContinue(context.Background(), &ListFolderContinueArg{Cursor: result.Cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResultPairs(result)).To(ConsistOf(
				listResultPair{MetadataDeleted, dir2.PathLower},
			))
			Expect(result.HasMore).To(BeFalse())
		})
	})

	Describe("CreateFolder", func() {
		It("should create folder", func() {
			name := randomName()

			md, err := client.CreateFolder(context.Background(), &CreateFolderArg{Path: "/" + name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			Expect(md.Tag).To(Equal(""))
		})
	})

	Describe("Delete", func() {
		It("should delete folder", func() {
			folder := createFolder()

			md, err := client.Delete(context.Background(), &DeleteArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))
			Expect(md.Tag).To(Equal("folder"))

			_, err = client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).To(HaveOccurred())
		})

		It("should fail to delete", func() {
			_, err := client.Delete(context.Background(), &DeleteArg{Path: "/somethingrandom"})
			Expect(err).To(HaveOccurred())

			dropboxErr, ok := IsDropboxError(err)
			Expect(ok).To(BeTrue())
			Expect(dropboxErr.Err.Tag).To(Equal("path_lookup"))
			Expect(dropboxErr.Err.PathLookup.Tag).To(Equal("not_found"))
		})
	})

	Describe("Copy", func() {
		It("should copy folder", func() {
			folder := createFolder()
			newName := fmt.Sprintf("%d", rand.Int())

			md, err := client.Copy(context.Background(), &RelocationArg{FromPath: "/" + folder.Name, ToPath: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))

			md, err = client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))

			md, err = client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))
		})
	})

	Describe("Move", func() {
		It("should move folder", func() {
			folder := createFolder()
			newName := fmt.Sprintf("%d", rand.Int())

			md, err := client.Move(context.Background(), &RelocationArg{FromPath: "/" + folder.Name, ToPath: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))

			_, err = client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).To(HaveOccurred())

			md, err = client.GetMetadata(context.Background(), &GetMetadataArg{Path: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))
		})
	})

	upload := func(name string) (res *Metadata, err error) {
		session, err := client.UploadSessionStart(context.Background(), strings.NewReader("123"))
		if err != nil {
			return nil, err
		}

		err = client.UploadSessionAppend(context.Background(), &UploadSessionCursor{SessionId: session.SessionId, Offset: 3}, strings.NewReader("45"))
		if err != nil {
			return nil, err
		}

		finishArg := &UploadSessionFinishArg{
			Cursor: &UploadSessionCursor{
				SessionId: session.SessionId,
				Offset:    5,
			},
			Commit: &CommitInfo{
				Path: "/" + name,
				Mode: &WriteMode{
					Tag: WriteModeAdd,
				},
				Autorename:     false,
				ClientModified: nil,
				Mute:           false,
			},
		}

		res, err = client.UploadSessionFinish(context.Background(), finishArg)
		if err != nil {
			return nil, err
		}

		return res, err
	}

	Describe("Download", func() {
		It("should download a file", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))

			reader, md, err := client.Download(context.Background(), &DownloadArg{Path: "/" + name}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			Expect(md.ETag).NotTo(Equal(""))
			Expect(md.ContentLength).To(Equal(int64(5)))

			data, _ := ioutil.ReadAll(reader)
			reader.Close()

			Expect(string(data)).To(Equal("12345"))
		})

		It("should download a file range", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			_ = ioutils.FileSpan{}

			reader, md, err := client.Download(context.Background(), &DownloadArg{Path: "/" + name}, &ioutils.FileSpan{2, 3})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			Expect(md.ETag).NotTo(Equal(""))
			Expect(md.ContentLength).To(Equal(int64(2)))

			data, _ := ioutil.ReadAll(reader)
			reader.Close()

			Expect(string(data)).To(Equal("34"))
		})
	})

	Describe("Upload", func() {
		It("should upload a file", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			Expect(md.Tag).To(Equal(""))
		})
	})
})
