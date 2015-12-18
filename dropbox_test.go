package dropboxclient

import (
	"fmt"
	"io/ioutil"
	"strings"
	"github.com/koofr/go-ioutils"
	"math/rand"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Dropbox", func() {
	var client *Dropbox

	accessToken := os.Getenv("DROPBOX_ACCESS_TOKEN")

	if accessToken == "" {
		fmt.Println("DROPBOX_ACCESS_TOKEN env variable missing")
		return
	}

	BeforeEach(func() {
		rand.Seed(time.Now().UnixNano())

		client = NewDropbox(accessToken)
	})

	var createFolder = func() *Metadata {
		name := fmt.Sprintf("%d", rand.Int())

		md, err := client.CreateFolder(&CreateFolderArg{Path: "/" + name})
		Expect(err).NotTo(HaveOccurred())
		Expect(md.Name).To(Equal(name))

		return md
	}

	Describe("GetSpaceUsage", func() {
		It("should get space usage", func() {
			usage, err := client.GetSpaceUsage()
			Expect(err).NotTo(HaveOccurred())
			Expect(usage.Allocation.Allocated).To(BeNumerically(">", 0))
			Expect(usage.Used).To(BeNumerically(">=", 0))
		})
	})

	Describe("GetMetadata", func() {
		It("should get metadata for path", func() {
			folder := createFolder()

			md, err := client.GetMetadata(&GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))
		})
	})

	Describe("ListFolder", func() {
		It("should list root", func() {
			createFolder()

			result, err := client.ListFolder(&ListFolderArg{Path: ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) > 0).To(BeTrue())
		})

		It("should list folder", func() {
			folder := createFolder()

			result, err := client.ListFolder(&ListFolderArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result.Entries) == 0).To(BeTrue())
		})
	})

	Describe("CreateFolder", func() {
		It("should create folder", func() {
			name := fmt.Sprintf("%d", rand.Int())

			md, err := client.CreateFolder(&CreateFolderArg{Path: "/" + name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
		})
	})

	Describe("Delete", func() {
		It("should delete folder", func() {
			folder := createFolder()

			md, err := client.Delete(&DeleteArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))

			_, err = client.GetMetadata(&GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Copy", func() {
		It("should copy folder", func() {
			folder := createFolder()
			newName := fmt.Sprintf("%d", rand.Int())

			md, err := client.Copy(&RelocationArg{FromPath: "/" + folder.Name, ToPath: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))

			md, err = client.GetMetadata(&GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(folder.Name))

			md, err = client.GetMetadata(&GetMetadataArg{Path: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))
		})
	})

	Describe("Move", func() {
		It("should move folder", func() {
			folder := createFolder()
			newName := fmt.Sprintf("%d", rand.Int())

			md, err := client.Move(&RelocationArg{FromPath: "/" + folder.Name, ToPath: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))

			_, err = client.GetMetadata(&GetMetadataArg{Path: "/" + folder.Name})
			Expect(err).To(HaveOccurred())

			md, err = client.GetMetadata(&GetMetadataArg{Path: "/" + newName})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(newName))
		})
	})

	upload := func(name string) (res *Metadata, err error) {
		session, err := client.UploadSessionStart(strings.NewReader("123"))
		if err != nil {
			return nil, err
		}

		err = client.UploadSessionAppend(&UploadSessionCursor{SessionId: session.SessionId, Offset: 3}, strings.NewReader("45"))
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

		res, err = client.UploadSessionFinish(finishArg)
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

			reader, md, err := client.Download(&DownloadArg{Path: "/" + name})
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))

			data, _ := ioutil.ReadAll(reader)
			reader.Close()

			Expect(string(data)).To(Equal("12345"))
		})
	})

	Describe("DownloadV1", func() {
		It("should download a file", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			_ = ioutils.FileSpan{}

			res, err := client.DownloadV1("/" + name, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ContentLength).To(Equal(int64(5)))

			data, _ := ioutil.ReadAll(res.Reader)
			res.Reader.Close()

			Expect(string(data)).To(Equal("12345"))
		})

		It("should download a file range", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
			_ = ioutils.FileSpan{}

			res, err := client.DownloadV1("/" + name, &ioutils.FileSpan{2, 3})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.ContentLength).To(Equal(int64(2)))

			data, _ := ioutil.ReadAll(res.Reader)
			res.Reader.Close()

			Expect(string(data)).To(Equal("34"))
		})
	})

	Describe("Upload", func() {
		It("should upload a file", func() {
			name := fmt.Sprintf("new-file-%d", rand.Int())

			md, err := upload(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(md.Name).To(Equal(name))
		})
	})
})
